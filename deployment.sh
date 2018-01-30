curl -d "type=note" -d "title=Deploment complete" -d "body=Really!" --header Access-Token:\ $PUSHBULLET_API_TOKEN https://api.pushbullet.com/v2/pushes >/dev/null
