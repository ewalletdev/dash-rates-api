package main

import (
	"fmt"
	"html/template"
	"io"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/labstack/echo"
	"github.com/labstack/echo/middleware"
	"github.com/parnurzeal/gorequest"
	"github.com/patrickmn/go-cache"
	"github.com/tidwall/gjson"
)

var cacheDuration = time.Minute

func main() {
	e := echo.New()

	loggerConfig := middleware.DefaultLoggerConfig
	loggerConfig.Skipper = func(c echo.Context) bool {
		return c.Path() == "/"
	}

	e.Use(middleware.LoggerWithConfig(loggerConfig))
	e.Use(middleware.Recover())
	e.Use(middleware.CORS())

	providers := new(providers)
	providers.cache = cache.New(cacheDuration, cacheDuration)

	routes := new(router)
	routes.providers = providers
	routes.templates = template.Must(template.ParseGlob("*.html"))

	e.GET("/", routes.Index)
	e.GET("/avg", routes.Average)
	e.GET("/poloniex", routes.Poloniex)
	e.GET("/btcaverage", routes.BTCAverage)
	e.GET("/invoice", routes.InvoiceViaCoinTigo)
	e.GET("/*", routes.Wildcard)
	e.Renderer = routes

	// go func() {
	// 	_, _ = providers.BitcoinaverageRates()
	// 	time.Sleep(cacheDuration)
	// }()
	//
	// go func() {
	// 	_, _ = providers.CryptocompareBTCDASHAverage()
	// 	time.Sleep(cacheDuration)
	// }()
	//
	// go func() {
	// 	_, _ = providers.DashCasaDASHVESRate()
	// 	time.Sleep(cacheDuration)
	// }()

	e.Logger.Fatal(e.Start(":3000"))
}

type router struct {
	providers *providers
	templates *template.Template
}

func (r *router) Render(w io.Writer, name string, data interface{}, c echo.Context) error {
	return r.templates.ExecuteTemplate(w, name, data)
}

// The documentation endpoint.
func (r *router) Index(c echo.Context) error {
	host := os.Getenv("HOST")
	if host == "" {
		host = "https://rates.dash-retail.com"
	}
	return c.Render(http.StatusOK, "apidoc.html", map[string]string{"host": host})
}

// The average BTC/DASH rate from various exchanges according to cryptocompare.com.
func (r *router) Average(c echo.Context) error {
	rate, err := r.providers.CryptocompareBTCDASHAverage()
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, rate)
}

// The average BTC/DASH rate calculated from the last 200 Poloniex trades.
func (r *router) Poloniex(c echo.Context) error {
	rate, err := r.providers.PoloniexBTCDASHAverage()
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, rate)
}

// The current BTC/DASH rate from bitcoinaverage.
func (r *router) BTCAverage(c echo.Context) error {
	rate, err := r.providers.BitcoinaverageCurrentBTCDASHRate()
	if err != nil {
		return err
	}
	return c.JSON(http.StatusOK, rate)
}

// Creates a CoinText invoice.
func (r *router) InvoiceViaCointext(c echo.Context) error {
	address := c.QueryParam("addr")
	amount, err := strconv.ParseInt(c.QueryParam("amount"), 10, 64)
	if err != nil || amount == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "Amount param is invalid")
	}

	// TODO: Restore the CoinText api request when we have a key...

	/*
	url := "https://pos-api.cointext.io/create_invoice/"

	_, body, errs := gorequest.New().Post(url).Send(map[string]interface{}{
		"address": address,
		"amount":  amount,
		"network": "dash",
		"api_key": os.Getenv("COINTEXT_API_KEY"),
	}).End()

	if len(errs) > 1 {
		broadcastErr(errs[0])
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to create CoinText invoice")
	}

	paymentId := gjson.Get(body, "paymentId").String()
	if paymentId == "" {
		err := echo.NewHTTPError(http.StatusInternalServerError, "Failed to determine CoinText paymentId")
		broadcastErr(err)
		return err
	}
	*/

	url := "https://api.get-spark.com/invoice"

	_, body, errs := gorequest.New().Get(url).Param("addr", address).Param("amount", fmt.Sprintf("%d", amount)).End()
	if len(errs) > 1 {
		broadcastErr(errs[0])
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to create CoinText invoice")
	}

	// fmt.Printf("%s - New invoice to %s for %d", c.RealIP(), address, amount)

	c.Logger().Printj(map[string]interface{}{
		"message":   "invoice",
		"remote_ip": c.RealIP(),
		"address":   address,
		"amount":    amount,
	})

	rsp := strings.Replace(body, `"`, "", -1)

	return c.JSON(http.StatusOK, rsp)
}

func (r *router) InvoiceViaCoinTigo(c echo.Context) error {
	url := "https://ctgoapi.ngrok.io/cointigo"

	address := c.QueryParam("addr")
	amount, err := strconv.ParseInt(c.QueryParam("amount"), 10, 64)
	if err != nil || amount == 0 {
		return echo.NewHTTPError(http.StatusBadRequest, "Amount param is invalid")
	}

	_, body, errs := gorequest.New().Post(url).Send(map[string]interface{}{
		"coin":    "DASH",
		"user":    "DaSh.OrG",
		"method":  "create_invoice",
		"address": address,
		"amount":  amount,
	}).End()

	if len(errs) > 1 {
		broadcastErr(errs[0])
		return echo.NewHTTPError(http.StatusInternalServerError, "Failed to create CoinText invoice")
	}

	// fmt.Printf("%s - New invoice to %s for %d", c.RealIP(), address, amount)

	c.Logger().Printj(map[string]interface{}{
		"message":   "invoice",
		"remote_ip": c.RealIP(),
		"address":   address,
		"amount":    amount,
	})

	rsp := gjson.Parse(body).Get("invoice").String()

	return c.JSON(http.StatusOK, rsp)
}

// The BTC rates from BitcoinAverage converted into DASH rates.
func (r *router) Wildcard(c echo.Context) error {
	s := c.Request().URL.Path
	s = strings.TrimPrefix(s, "/")
	s = strings.TrimSuffix(s, "/")
	s = strings.ToUpper(s)

	var selectedCurrencies []string

	supportedCurrencies := []string{
		"AED", "AFN", "ALL", "AMD", "ANG", "AOA", "ARS", "AUD", "AWG", "AZN", "BAM", "BBD", "BDT", "BGN",
		"BHD", "BIF", "BMD", "BND", "BOB", "BRL", "BSD", "BTN", "BWP", "BYN", "BZD", "CAD", "CDF", "CHF", "CLF", "CLP",
		"CNH", "CNY", "COP", "CRC", "CUC", "CUP", "CVE", "CZK", "DJF", "DKK", "DOP", "DZD", "EGP", "ERN", "ETB", "EUR",
		"FJD", "FKP", "GBP", "GEL", "GGP", "GHS", "GIP", "GMD", "GNF", "GTQ", "GYD", "HKD", "HNL", "HRK", "HTG", "HUF",
		"IDR", "ILS", "IMP", "INR", "IQD", "IRR", "ISK", "JEP", "JMD", "JOD", "JPY", "KES", "KGS", "KHR", "KMF", "KPW",
		"KRW", "KWD", "KYD", "KZT", "LAK", "LBP", "LKR", "LRD", "LSL", "LYD", "MAD", "MDL", "MGA", "MKD", "MMK", "MNT",
		"MOP", "MRO", "MUR", "MVR", "MWK", "MXN", "MYR", "MZN", "NAD", "NGN", "NIO", "NOK", "NPR", "NZD", "OMR", "PAB",
		"PEN", "PGK", "PHP", "PKR", "PLN", "PYG", "QAR", "RON", "RSD", "RUB", "RWF", "SAR", "SBD", "SCR", "SDG", "SEK",
		"SGD", "SHP", "SLL", "SOS", "SRD", "SSP", "STD", "SVC", "SYP", "SZL", "THB", "TJS", "TMT", "TND", "TOP", "TRY",
		"TTD", "TWD", "TZS", "UAH", "UGX", "USD", "UYU", "UZS", "VES", "VND", "VUV", "WST", "XAF", "XAG", "XAU", "XCD",
		"XDR", "XOF", "XPD", "XPF", "XPT", "YER", "ZAR", "ZMW", "ZWL",
	}

	if strings.Index(s, "LIST") != 0 {
		o := regexp.MustCompile(`^(/?[A-Z]{3})*$`).MatchString(s)
		if o == false {
			return echo.NewHTTPError(http.StatusBadRequest, "Malformed currency selection in url")
		}
		selectedCurrencies = strings.Split(s, "/")

		for _, selectedCurrency := range selectedCurrencies {
			i := sort.SearchStrings(supportedCurrencies, selectedCurrency)
			if supportedCurrencies[i] != selectedCurrency {
				return echo.NewHTTPError(http.StatusBadRequest, "Unsupported currency selection in url")
			}
		}
	} else {
		selectedCurrencies = supportedCurrencies
	}

	btcRates, err := r.providers.BitcoinaverageRates()
	if err != nil {
		return err
	}

	btcDashRate, err := r.providers.CryptocompareBTCDASHAverage()
	if err != nil {
		return err
	}

	if btcDashRate == 0 {
		btcDashRate, err = r.providers.BitcoinaverageCurrentBTCDASHRate()
		if err != nil {
			return err
		}
	}

	rates := make(map[string]float64)

	for _, currency := range selectedCurrencies {
		if currency == "VES" {
			rates["VES"], err = r.providers.DashCasaDASHVESRate()
			if err != nil {
				return err
			}
		} else {
			rates[currency] = btcRates[currency] * btcDashRate
		}
	}

	c.Logger().Printj(map[string]interface{}{
		"message":   "rates",
		"remote_ip": c.RealIP(),
		"rates":     rates,
	})

	return c.JSON(http.StatusOK, rates)
}

type providers struct {
	cache *cache.Cache
}

func (p *providers) CryptocompareBTCDASHAverage() (rate float64, err error) {
	url := "https://min-api.cryptocompare.com/data/generateAvg?fsym=DASH&tsym=BTC&e=Binance,Kraken,Poloniex,Bitfinex"
	rateI, found := p.cache.Get(url)
	if !found {
		fmt.Println("Recaching CryptocompareBTCDASHAverage")
		_, body, errs := gorequest.New().Get(url).End()
		if len(errs) > 1 {
			broadcastErr(errs[0])
			err = echo.NewHTTPError(http.StatusInternalServerError, "Failed to fetch BTCDASH rate from CryptoCompare")
			return
		}
		rate, err = strconv.ParseFloat(gjson.Get(body, "RAW.PRICE").String()[1:], 10)
		p.cache.SetDefault(url, rate)
	} else {
		rate, _ = rateI.(float64)
	}
	return
}

func (p *providers) PoloniexBTCDASHAverage() (rate float64, err error) {
	url := "https://poloniex.com/public?command=returnTradeHistory&currencyPair=BTC_DASH"
	rateI, found := p.cache.Get(url)
	if !found {
		fmt.Println("Recaching PoloniexBTCDASHAverage")
		_, body, errs := gorequest.New().Get(url).End()
		if len(errs) > 1 {
			broadcastErr(errs[0])
			err = echo.NewHTTPError(http.StatusInternalServerError, "Failed to fetch BTCDASH average rate from Poloniex")
			return
		}
		rateTotal := 0.0
		rateCount := 0
		gjson.Get(body, "#.rate").ForEach(func(key, value gjson.Result) bool {
			rateTotal += value.Float()
			rateCount += 1
			return true
		})
		rate = rateTotal / float64(rateCount)
		p.cache.SetDefault(url, rate)
	} else {
		rate, _ = rateI.(float64)
	}
	return
}

func (p *providers) BitcoinaverageCurrentBTCDASHRate() (rate float64, err error) {
	url := "https://apiv2.bitcoinaverage.com/indices/crypto/ticker/DASHBTC"
	rateI, found := p.cache.Get(url)
	if !found {
		fmt.Println("Recaching BitcoinaverageCurrentBTCDASHRate")
		_, body, errs := gorequest.New().Get(url).End()
		if len(errs) > 1 {
			broadcastErr(errs[0])
			err = echo.NewHTTPError(http.StatusInternalServerError, "Failed to fetch BTCDASH rate from BitcoinAverage")
			return
		}
		rate = gjson.Get(body, "last").Float()
		p.cache.SetDefault(url, rate)
	} else {
		rate, _ = rateI.(float64)
	}
	return
}

func (p *providers) BitcoinaverageRates() (rates map[string]float64, err error) {
	url := "https://apiv2.bitcoinaverage.com/indices/global/ticker/short?crypto=BTC"
	ratesI, found := p.cache.Get(url)
	if !found {
		fmt.Println("Recaching BitcoinaverageRates")
		rates = make(map[string]float64)
		_, body, errs := gorequest.New().Get(url).End()
		if len(errs) > 1 {
			broadcastErr(errs[0])
			err = echo.NewHTTPError(http.StatusInternalServerError, "Failed to fetch rates from BitcoinAverage")
			return
		}

		gjson.Parse(body).ForEach(func(key, value gjson.Result) bool {
			rates[key.String()[3:]] = value.Get("last").Float()
			return true
		})

		p.cache.SetDefault(url, rates)
	} else {
		rates, _ = ratesI.(map[string]float64)
	}

	return
}

func (p *providers) DashCasaDASHVESRate() (rate float64, err error) {
	url := "http://dash.casa/api/?cur=VES"
	rateI, found := p.cache.Get(url)
	if !found {
		fmt.Println("Recaching DashCasaDASHVESRate")
		_, body, errs := gorequest.New().Get(url).End()
		if len(errs) > 1 {
			broadcastErr(errs[0])
			err = echo.NewHTTPError(http.StatusInternalServerError, "Failed to fetch DASHVES rate from Dash Casa")
			return
		}
		rate = gjson.Get(body, "dashrate").Float()
		p.cache.SetDefault(url, rate)
	} else {
		rate, _ = rateI.(float64)
	}
	return
}

func broadcastErr(err error) {
	webhookUrl := os.Getenv("DISCORD_WEBHOOK_URL")
	if webhookUrl != "" {
		jsn := `
{
  "username": "Dash Rates API",
  "embeds": [
    {
      "title": "ERROR",
      "description": "` + err.Error() + `",
      "color": 15340307
    }
  ]
}
	`

		gorequest.
			New().
			AppendHeader("content-type", "application/json").
			Post(webhookUrl).
			Send(jsn).
			End()
	}
}
