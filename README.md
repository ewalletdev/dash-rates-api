# Dash Rates API

A Golang implementation of the [NodeJS Dash Rates API](https://github.com/kodaxx/DashRates-API) created by Kodaxx.

The [CoinText](https://cointext.io/en) `COINTEXT_API_KEY` environment variable must be set for the `/invoice` endpoint to work.

There is no requirement to run Redis.

API documentation can be viewed by visiting the root url.

## Running
Set the following optional environment variables as required:
```
export COINTEXT_API_KEY="..."
export DISCORD_WEBHOOK_URL="https://discordapp.com/api/webhooks/..."
export HOST="https://your-domain.com"
```

Run the dockerhub image:
```
docker run \
  -d \
  -p 3000:3000 \
  -e COINTEXT_API_KEY="$COINTEXT_API_KEY" \
  -e DISCORD_WEBHOOK_URL="$DISCORD_WEBHOOK_URL" \
  -e HOST="$HOST" \
  --name dash-rates-api \
  ewalletdev/dash-rates-api:v1
```

## Building
To build the docker container from source as opposed to using the dockerhub image run the following commands:
```
git clone https://github.com/ewalletdev/dash-rates-api.git
cd dash-rates-api
docker build -t ewalletdev/dash-rates-api:v1 .
```
