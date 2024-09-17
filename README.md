# inmate
Inmate is the Influxdb v1 data sync tool for partial fault tolerance.

InfluxDB OSS operates as a single-instance database. The Inmate project enhances data resilience by allowing multiple InfluxDB nodes to run concurrently. However, it comes with some limitations:

No Backfilling: If a new InfluxDB address is introduced, Inmate cannot backfill existing data.
Data Loss Risk: If an instance is unreachable for an extended period, data loss may still occur. You can mitigate this by configuring retry counts, delays, and queue sizes through the config.yaml.

No Data Partitioning: Inmate does not partition data, which is a critical feature for true database clustering.

In summary, the Inmate project enables data synchronization across multiple InfluxDB instances, covers brief downtimes, and balances queries between nodes. However, it is not a full-fledged clustering solution. For that, InfluxDB Enterprise is required. Nonetheless, Inmate provides a degree of increased reliability and confidence.

## Usage:

### Building the app

    go build .

### Building with docker

    docker build .

### Testing with docker compose

    docker compose up --build --force-recreate

Exposes the API over port `8080`. Connect with `influx -port 8080`

## Note

All headers are directly forwarded to the influxdb instances including Auth headers.
Therefore, there is no need to explicitly configure the auth credentials inside the inmage.

`config.yaml` includes `influxdb3` on purpose. I am testing the "down node" scenario with that config.
