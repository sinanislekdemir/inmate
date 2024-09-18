import requests
import time
from datetime import datetime

# Configuration for InfluxDB v2
INFLUXDB_URL = 'http://localhost:8080'    # Replace with your InfluxDB URL
ORG = 'myorg'                             # Replace with your organization name
BUCKET = 'temp'                           # Replace with your bucket name
TOKEN = '123456'                          # Replace with your authentication token

HEADERS = {
    'Authorization': f'Token {TOKEN}',
    'Content-Type': 'text/plain; charset=utf-8'
}

# Escape special characters in tag values (spaces, commas, and equals signs)
def escape_tag_value(value):
    return value.replace(" ", "\\ ").replace(",", "\\,").replace("=", "\\=")

# Data Generation
def generate_data_points(start_timestamp, num_points):
    data_points = []
    cities = ["New York", "London", "Paris", "Tokyo", "Berlin"]  # Example cities
    for i in range(num_points):
        city = escape_tag_value(cities[i % len(cities)])
        temperature = 20.0 + i % 5  # Varying temperature values
        timestamp = start_timestamp + i * 1_000_000_000  # 1 second intervals in nanoseconds
        line = f"weather,city={city} temperature={temperature} {timestamp}"
        data_points.append(line)
    return data_points

# Send data to InfluxDB
def send_batch_to_influxdb(batch):
    data = '\n'.join(batch)
    params = {
        'org': ORG,
        'bucket': BUCKET,
        'precision': 'ns'
    }
    response = requests.post(INFLUXDB_URL + "/api/v2/write", headers=HEADERS, params=params, data=data)
    if response.status_code == 204:
        print("Batch successfully written to InfluxDB.")
    else:
        print(f"Failed to write batch: {response.status_code}, {response.text}")

# Query data for London using Flux
def query_data_for_london():
    query = f'''
    from(bucket: "{BUCKET}")
        |> range(start: -1h)
        |> filter(fn: (r) => r._measurement == "weather" and r.city == "London")
    '''
    params = {
        'org': ORG
    }
    response = requests.post(INFLUXDB_URL + "/api/v2/query", headers={
        'Authorization': f'Bearer {TOKEN}',
        'Content-Type': 'application/vnd.flux',
        'Accept': 'application/json',
    }, params=params, data=query)
    
    if response.status_code == 200:
        print(response.content)
    else:
        print(f"Failed to query data: {response.status_code}, {response.text}")

# Main execution
if __name__ == '__main__':
    # Start timestamp from current time
    start_datetime = datetime.now()
    start_timestamp_ns = int(start_datetime.timestamp() * 1_000_000_000)

    # Generate 10,000 data points
    total_data_points = 10_000
    data_points = generate_data_points(start_timestamp_ns, total_data_points)

    # Send data in batches of 1,000
    batch_size = 1_000
    for i in range(0, len(data_points), batch_size):
        batch = data_points[i:i + batch_size]
        send_batch_to_influxdb(batch)

    # Query data after sending all batches
    query_data_for_london()
