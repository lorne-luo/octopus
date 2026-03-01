#!/bin/bash


# curl -X POST http://localhost:8080/v1/chat/completions \
#     -H "Content-Type: application/json" \
#     -H "Authorization: Bearer sk-octopus-lorne" \
#     -d @req.json

curl -X POST http://localhost:8080/v1/responses \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer sk-octopus-lorne" \
    -d @req.json

curl -X POST http://localhost:8080/v1/messages \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer sk-octopus-lorne" \
    -d @req.json


curl -X POST http://localhost:8080/v1beta/models/gemini-2.5-flash:generateContent \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer sk-octopus-lorne" \
    -d @req.json

curl -X POST "https://generativelanguage.googleapis.com/v1beta/models/gemini-2.5-flash:generateContent" \
     -H "x-goog-api-key: AIzaSyD7EQ-Q7SP2x3YVBDa521XENev59foSkm4" \
     -H "Content-Type: application/json" \
     -d '{
           "contents": [
             {
               "parts": [
                 {
                   "text": "hi"
                 }
               ]
             }
           ]
         }'