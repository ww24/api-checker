# api-checker

Check API response and notify.

## Usage

```
curl -XPOST -H"Content-Type: application/json" http://localhost:8080 -d @payload.json
```

### payload.json

```json
{
    "url": "https://ipinfo.io/products/api/ip-geolocation-api?value=120.51.198.59",
    "method": "POST",
    "content_type": "",
    "body": "",
    "query": ".data.city == \"Tokyo\"",
    "notification_message": "ip address located in Tokyo"
}
```
