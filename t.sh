curl https://apis.iflow.cn/v1/chat/completions \
    -H "Content-Type: application/json" \
    -H "Authorization: Bearer sk-7075ce9e73c2fe456f0e26d797c2cc98" \
    -d '{
      "model": "glm-4.7",
      "messages": [
        {"role": "user", "content": "你好，请介绍一下你自己"}
      ],
      "stream": true
    }'