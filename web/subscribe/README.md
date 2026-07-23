# Mochi 网页认购端

独立于 `desktop/` 的 HTML5 认购站点，对接同一后端 API。

## 启动

```bash
# 1. 后端（项目根目录）
cd server
$env:CONFIG_PATH = "d:\ocr\Mochi\config.yaml"
.\mochi-server.exe

# 2. 认购 Web
cd web/subscribe
npm install
npm run dev
```

浏览器打开：**http://127.0.0.1:5173**

## 流程

1. `/login` — 注册 / 登录
2. `/catalog` — 图鉴选 SKU → 免费认领
3. `/success` — 认购成功 → 下载客户端引导

## 端口

| 项目 | 端口 |
|------|------|
| 后端 | 8081 |
| 认购 Web | **5173** |
| 桌宠 desktop | 1420（勿与 tauri dev 冲突） |
