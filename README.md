# inmate
Inmate is the Influxdb v1 and v2 data sync tool for partial fault tolerance.

InfluxDB OSS operates as a single-instance database. The Inmate project enhances data resilience by allowing multiple InfluxDB nodes to run concurrently. However, it comes with some limitations:

No Backfilling: If a new InfluxDB address is introduced, Inmate cannot backfill existing data.
Data Loss Risk: If an instance is unreachable for an extended period, data loss may still occur. You can mitigate this by configuring retry counts, delays, and queue sizes through the config.yaml.

No Data Partitioning: Inmate does not partition data, which is a critical feature for true database clustering.

In summary, the Inmate project enables data synchronization across multiple InfluxDB instances, covers brief downtimes, and balances queries between nodes. However, it is not a full-fledged clustering solution. For that, InfluxDB Enterprise is required. Nonetheless, Inmate provides a degree of increased reliability and confidence.

## Design

The design of Inmate is straightforward. All endpoints such as `/write`, `/query`, `/api/v2/write`, and `/ping` are proxied to the InfluxDB instances.

- **Write Requests:** Incoming write requests are buffered in dedicated channels for each InfluxDB instance. If an instance is unresponsive, its specific messages remain in the buffer until it can respond.
  
- **Read Requests:** For read requests (excluding mutations like creating or altering databases), queries are routed to a random instance in the pool. If the selected instance is unresponsive or returns an error, Inmate falls back to the next instance, continuing this process until all instances in the pool are exhausted.

- **Mutation Queries:** When handling mutation queries, the system waits for responses from all instances. If any instance is down, this could introduce a delay. In the worst-case scenario, if no response is received, an error will be raised.

In summary, Inmate functions as an InfluxDB gateway, distributing requests to independently running InfluxDB instances.

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

### Influxdb V2

Attention! Not all API endpoints are supported by inmate. But it will give you at least read/write operations fine.

## Configuration notes

In the configuration, the token is required only for InfluxDB v2. For InfluxDB v1, you can use the same authentication for all instances, as authorization parameters are directly forwarded to the InfluxDB instances. However, this is not possible with v2, as it generates random tokens that cannot be modified. Therefore, you must explicitly define tokens for each instance in the config.

Additionally, you can set a global `auth_token` in the configuration. If you prefer not to hardcode your token (protecting Inmate with `Authorization: Token <token>`), you can define it using the `AUTH_TOKEN` environment variable.

### Example config

    # Please generate your own tokens in each instance separately and replace it with the example token below
    # The tokens below are just examples and will not work
    # You can generate from the influxdb UI by http://localhost:8086 for influxdb1 and http://localhost:8087 for influxdb2
    
    addresses:
      - url: http://influxdb1:8086
        token: bGFgMnt42CrFEKO2mNHIAX55Qp8ENrLI643QHGZ-Uwo7pgSdYKt1_S7aqcpMYS7qySsOXSP-z6Ip3rGjVeLVvA==
      - url: http://influxdb2:8086
        token: KXlUtzPwoWvhDA-d-BqGHMp4O-stQrdLn6AyftmzOo_WZ_IOIUkMs2TB8sBHz7t28_oKQlFo1prlyRI2OMrV1w==
    bind_address: 0.0.0.0
    port: 8080
    retry_count: 120
    retry_delay: 1
    query_timeout: 5
    channel_size: 1000
    auth_token: 123456

### Example request

    curl http://localhost:8080/health -v -H "Authorization: Token 123456"