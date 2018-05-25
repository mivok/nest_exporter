#!/bin/bash

# Go to console.developers.nest.com (sign up at developers.nest.com if
# necessary) and create a new oauth client, leaving the default oauth redirect
# URI blank for pin based authorization. Note down the client ID and secret
# and then set CLIENT_ID and CLIENT_SECRET here:
CLIENT_ID=""
CLIENT_SECRET=""

AUTH_URL="https://home.nest.com/login/oauth2?client_id=$CLIENT_ID&state=STATE"

# View Auth URL in your browser
open "$AUTH_URL"

read -r -p "Enter code given to you by nest: " CODE

curl -X POST \
    -d "client_id=$CLIENT_ID" \
    -d "client_secret=$CLIENT_SECRET" \
    -d "code=$CODE" \
    -d "grant_type=authorization_code" \
    https://api.home.nest.com/oauth2/access_token | jq -r .access_token
