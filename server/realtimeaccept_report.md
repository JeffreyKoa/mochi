# 实时对话 Phase2 验收报告

时间: 2026-08-07T09:48:32+08:00

**自动化: 1/3 通过**

| 检查项 | 结果 | 说明 |
|--------|------|------|
| realtime 单元测试 | ✅ | ok  	github.com/mochi-ai/server/internal/realtime	2.794s |
| Phase2 配置项 | ❌ | enabled=true silence=700 endpointing=false filler=false |
| WS 文本 3 轮无崩溃 | ❌ | status 500: {"error":"Error 1054 (42S22): Unknown column 'tts_mode' in 'field list'"} |
| STT endpointing P50<600ms | ⏳人工 | 桌面+麦克风：判停 audio_end 应 300–600ms |
| Thinking filler 垫话 | ⏳人工 | 语音轮次 LLM>800ms 时 1s 内有垫话 |
| Barge-in 打断 | ⏳人工 | TTS 播放时说话可打断 |
| 5 轮连续语音 | ⏳人工 | 无崩溃、无整轮 ASR/TTS 失败 |
| playback 埋点 ≥4/5 | ⏳人工 | latency 日志 playback 非 -1 |
