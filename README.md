# inmate
Inmate is the Influxdb v1 data sync tool for partial fault tolerance.

## Usage:

### Building the app

    go build .

### Building with docker

    docker build .

### Testing with docker compose

    docker compose up

Exposes the API over port `8080`. Connect with `influx -port 8080`

## Note

All headers are directly forwarded to the influxdb instances including Auth headers.
Therefore, there is no need to explicitly configure the auth credentials inside the inmage.
