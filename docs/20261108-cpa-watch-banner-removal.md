# 2026-11-08 移除 Console 顶部 cpa-watch 健康告警条

## 背景
用户多次反馈 Console 顶部仍显示 "● 1 个账号曾被停服，已禁用处理 查看详情 →"，
但在 Console 前端代码 (internal/console/) 内反复搜索均无结果。

## 根因
该横幅**不是** Console 前端渲染的，而是由东京机 openresty (nginx) 通过
`sub_filter` 在 HTTP 响应体层注入的，与 CPA 二进制和前端代码完全解耦：

注入链路（全部在东京机 /etc/nginx 侧，不在 git 仓内）：
1. `conf.d/91-cpa-badge-map.conf` —— map 定义 `$cpa_badge_html` 常量片段
2. `www/sites/cpa.tannerlab.xyz/proxy/console-inject.conf` —— 对 `location = /console`
   和 `/console/` 执行 `sub_filter "</body>" "$cpa_badge_html</body>"`
3. `www/cpa-watch/badge.html` + `badge.json` —— 由 `cpa-silent-death-watch.py`
   每 15 分钟生成
4. `conf.d/90.cpa-watch.conf` + `www/sites/cpa.tannerlab.xyz/proxy/cpa-watch-domain.conf`
   —— 提供 /cpa-watch/ 详情页

因此：改前端代码、清浏览器缓存、重新编译二进制、重新部署容器，
**全部无法**移除该横幅。只有改 nginx 配置并 reload 才有效。

## 处置（2026-11-08 09:21 东京机执行）
- 备份至 /root/backup-cpa-watch-removal-20261108/
- console-inject.conf / cpa-watch-domain.conf 改名加 .removed-20261108 后缀
- 90.cpa-watch.conf / 91-cpa-badge-map.conf 移至 conf.d/disabled/
- www/cpa-watch/ 改名为 www/cpa-watch.removed-20261108/
- `systemctl disable --now cpa-silent-death-watch.timer`
- nginx -t 通过后 reload
- 验证：curl /console/ 与经 Cloudflare 的 https://cpa.tannerlab.cn/console/
  对 `cpa-watch-badge` 均 0 命中，字节数 109788 不变

## 教训
生产环境的"前端看起来是前端问题"，可能是反向代理层注入的。
排查顺序应该是：先看响应体原文（curl | grep），再怀疑前端代码。
本次前几轮把力气全花在 grep JS 文件上是无效功。
