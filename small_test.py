import requests
import time
from datetime import datetime, timedelta

# Configuration for InfluxDB v1.8
INFLUXDB_URL = 'http://localhost:8080'  # Replace with your InfluxDB URL
DATABASE = 'mydb'                             # Replace with your database name
USERNAME = 'admin'                            # Replace with your username (if applicable)
PASSWORD = 'password'                         # Replace with your password (if applicable)
 
HEADERS = {
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
        'db': DATABASE,
        'u': USERNAME,
        'p': PASSWORD,
        'precision': 'ns'
    }
    response = requests.post(INFLUXDB_URL + "/write", headers=HEADERS, params=params, data=data)
    if response.status_code == 204:
        print("Batch successfully written to InfluxDB.")
    else:
        print(f"Failed to write batch: {response.status_code}, {response.text}")

def create_database():
    params = {
        'q': f"CREATE DATABASE {DATABASE}",
        'u': USERNAME,
        'p': PASSWORD,
    }
    response = requests.post(INFLUXDB_URL + "/query", headers=HEADERS, params=params)
    if response.status_code == 200:
        print("Database created successfully.")
    else:
        print(f"Failed to create database: {response.status_code}, {response.text}")


# Main execution
if __name__ == '__main__':
    # Start timestamp from current time
    start_datetime = datetime.now()
    start_timestamp_ns = int(start_datetime.timestamp() * 1_000_000_000)

    # create_database()

    # Generate 10,000 data points
    total_data_points = 10_000
    data_points = generate_data_points(start_timestamp_ns, total_data_points)

    # Send data in batches of 1,000
    batch_size = 1_000
    for i in range(0, len(data_points), batch_size):
        batch = data_points[i:i + batch_size]
        send_batch_to_influxdb(batch)
        time.sleep(1)  # Optional delay to avoid overloading the server
