FROM golang:alpine
WORKDIR /go/src/go.ewallet.co.uk/dash-rates-api
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -a -installsuffix cgo -o app
RUN apk add ca-certificates

FROM scratch
WORKDIR /
COPY --from=0 /go/src/go.ewallet.co.uk/dash-rates-api/app /
COPY --from=0 /go/src/go.ewallet.co.uk/dash-rates-api/apidoc.html /
COPY --from=0 /etc/ssl/certs /etc/ssl/certs
ENTRYPOINT ["/app"]