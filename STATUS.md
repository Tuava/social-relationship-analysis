# 项目状态

本仓库包含 Go API、MCP stdio 服务、Vue 3 工作台和 QZone bridge 源码。

发布验证以 [GitHub Actions](https://github.com/Tuava/social-relationship-analysis/actions) 的实际运行结果为准；工作站上的历史采集统计不作为通用验收依据。

- 生产包：Linux/macOS，amd64/arm64；包含后端、MCP、时间窗文字采集 CLI、前端静态资源和数据库迁移。
- QZone bridge 源码随仓库提供，保留上游 MIT 许可与本项目修改；属于可选外部服务。
- NapCat/QQ、PostgreSQL、账号会话与模型服务由部署环境提供，不包含在可执行包中。
- 真实账号/网络/模型端到端验证需要另行配置；CI 使用临时数据库和合成测试数据。

详见 [发布审查](docs/release-review.md)、[安装指南](docs/release-install.md)。
