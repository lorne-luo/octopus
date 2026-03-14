我正在编写一个openai anthropic gemini三家llm api转换的proxy app
关于他们api request和response的相互转化,我制定了这些.md 计划文档

我希望你参考三家llm provider的官方文档, 重新检查一下docs/plans/transformer_canonical_rewrite这份计划有何缺失,要考虑
- streaming 响应
- 工具调用,function call
- 图片等media调用
- thinking,reasoning 开启
- used token返回

documents references:
- openai chat/completions: 
  - https://developers.openai.com/api/reference/resources/chat/subresources/completions/methods/create
  - https://platform.openai.com/docs/guides/reasoning
  - https://platform.openai.com/docs/guides/function-calling

- anthropic: 
  - https://platform.claude.com/docs/en/api/messages/create
  - https://platform.claude.com/docs/en/api/messages/count_tokens
  - https://docs.anthropic.com/en/docs/build-with-claude/prompt-caching

- gemini: 
  - https://ai.google.dev/gemini-api/docs/gemini-3  
  - https://ai.google.dev/gemini-api/docs/text-generation 
  - https://ai.google.dev/api/generate-content
  - https://ai.google.dev/gemini-api/docs/function-calling

不要过度计划,力求简单实现,但所有claude code需要的功能,比如工具调用必须要纳入计划

如果发现什么值得修改,先不修改文件,与我沟通

--- 

/ralph-wiggum:ralph-loop 根据docs/transformer_canonical_rewrite的设计文档,实现新的internal/transformer2,要求采用TDD跑通docs/transformer_canonical_rewrite/tests里的测试用例,针对文档中每个功能点调用subaggent sonnet-agent 对其进行开发,每完成一个功能点都要更新progress.md并提交到git,每完成一个功能点都清空上下文 --completion-promise "DONE" --max-iterations 3