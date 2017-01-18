# drone-rancher

Unofficial DroneCI / Rancher integration with better deployment control and failure notifications via Slack.

## Configuration

````
pipeline:
  rancher:
    image: buildertools/drone-rancher
    url: <your rancher endpoint as reachable from a drone node>
    timeout: 5m
    access_key: <please use Drone secrets>
    secret_key: <please use Drone secrets>
    service: <service name>
    stack: <stack name>
    docker_image: <repo:tag for deployment>
    confirm: false 
    start_first: true
    batch_size: 1
    batch_interval: 1m <standard duration format>
    notify_webhook: <your slack webhook>
    success_emoji: thumbsup
    blocked_emoji: boom
    success_channel: builds
    blocked_channel: builds
````

Note that ````notify_webhook````, ````success_emoji````, ````blocked_emoji````, ````success_channel````, and ````blocked_channel```` are all optional fields.

