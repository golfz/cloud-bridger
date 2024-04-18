# Cloud Bridger

A cloud-based service that receives HTTP requests, converts them into JSON messages, and sends them to a local bridger
via a WebSocket.

This service works with the "Local Bridger" tool you can find on GitHub here: https://github.com/golfz/local-bridger.

## Configuration

- see `config.yaml` for configuration options
- for Environment Variables, replace `.` with `_` and use all caps

### Environment Variables

**Cloud Server**

| Environment Variable | Description                  | Default | Comment  |
|----------------------|------------------------------|---------|----------|
| `SERVICE_PORT`       | the port for running service | `5000`  | required |

## Running

```shell
SERVICE_PORT=5000 go run application.go
```

## Call API to a private local server

You can call the API using any HTTP method, HTTP headers, path parameters, query parameters, and body, just as you would
with a
typical API.

However, you must include the HTTP header `X-Private-Server-ID` with the same value you set in "Local Bridger."

For example, If you set the `X-Private-Server-ID` to `my-private-server` in "Local Bridger," you must include the same
value in the header when calling the API.

For this example, let's assume that your Cloud Bridger service is hosted at https://example.com.

```shell
curl -X POST https://example.com:5000/api/v1/echo -H "X-Private-Server-ID: my-private-server" -d '{"message": "Hello, World!"}'
```
