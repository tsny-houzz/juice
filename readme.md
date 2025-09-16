# Juice

A log reader. Designed to read jukwaa-like apps JSON output from k8s.

JSON lines are parsed and things like severity, url, message are logged.

# Examples

```sh
❯ NAMESPACE=debug juice -pod jukwaa-74cb6c4bdf-nttkv
...
[info] [/] connected to client feat_eval_client
[info] [/] FeatureEvalService featCtx
[info] [/] FeatureEvalService result
codespace:: adding cookie csdebug
Adding cookie to thrift header:  csdebug=main-jukwaa-release20250915161114cbfa441278
[winston] Attempt to write logs with no transports, which can increase memory usage: {"level":"debug","message":"starting thrif
t connection"}
[winston] Attempt to write logs with no transports, which can increase memory usage: {"level":"debug","message":"thrift connect
ion established: c2-thrift.codespace.svc.cluster.local:8094"}
[info] [/] front end middleware debug fb-auth-migration
[info] [/] front end middleware debug email-verification-hardgate started
[info] [/] front end middleware debug email-verification-hardgate check if get current user service is working
[info] [/] front end middleware debug pro-on-hold-hardgate started
[info] [/] front end middleware debug pro on hold hardgate commandName and pageName:
[info] [/] Tunnel Handler serving homepage
GET /  | - ?ms |  |
[info] [/] Jukwaa C2Csrf validation: signedIn->false h0-> h1->webuser_985512891757699323
codespace:: adding cookie csdebug
```

# Under the hood

JSON Struct:
```json
{
  "application": "jukwaa-mono",
  "component": "jukwaa-main-access",
  "environment": {
    "arch": "aarch64",
    "cluster": "",
    "pool": "main",
    "server": "jukwaa-main-release202509101901175f64dafc46-66f8b4f8b8-2pp4z",
    "version": ""
  },
  "level": "access",
  "levelMeta": {},
  "logVersion": "v2",
  "message": "",
  "metadata": {
    "client-ip": "66.249.79.4",
    "command-name": "viewGallery",
    "date": "16/Sep/2025:00:43:48 +0000",
    "domain": "www.houzz.ru",
    "geo": {
      "addr": "66.249.79.4",
      "city": "",
      "country": "US",
      "dma": "",
      "postalCode": "",
      "region": "",
      "timeZone": "America/Chicago"
    },
    "http-version": "HTTP/1.1",
    "method": "GET",
    "remote-addr": "66.249.79.4, 66.249.79.4, 167.82.143.31,10.30.2.166",
    "request-id": "ea668491-5d5e-4907-aa0f-031066aa1068",
    "response-content-length": "",
    "response-timeMS": "468.788",
    "status": "200",
    "url": "/statyi/9-cocinas-de-estilo-rustico-practicas-y-actuales-stsetivw-vs~95688411",
    "user-agent": "Mozilla/5.0 (Linux; Android 6.0.1; Nexus 5X Build/MMB29P) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.7339.127 Mobile Safari/537.36 (compatible; GoogleOther)"
  },
  "spanId": "56c968c2c287f6d9",
  "stack": "",
  "timestamp": 1757983428153,
  "traceId": "9e9e7a53d85f7581692f9ab2ecec3f19"
}
```