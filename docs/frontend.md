# 前端与 Vuetify 设计

## 技术栈

```text
Vue 3
TypeScript
Vite
Vuetify 3
Pinia
Vue Router
Cytoscape.js
ECharts
ofetch
```

## Vuetify 的职责

Vuetify 负责通用分析工作台界面：

- `VApp`、`VLayout`、`VNavigationDrawer`：应用框架和导航；
- `VDataTable`：用户、群、消息、关系事件和证据列表；
- `VTextField`、`VSelect`、`VDateInput`：目标账号、数据源和时间范围筛选；
- `VCard`、`VList`、`VTimeline`：摘要、资料版本和事件时间线；
- `VDialog`、`VBottomSheet`、`VOverlay`：证据详情和任务状态；
- `VChip`、`VBadge`、`VProgressLinear`：来源、关系类型、置信度和采集进度；
- 主题系统：深色优先的分析工作台，避免使用营销型大卡片布局。

Cytoscape.js 专门负责关系图画布，不能用大量 Vuetify 卡片模拟图谱；ECharts 专门负责时间趋势、分布和统计图。

## 页面边界

```text
分析工作台
├── 目标账号与任务配置
├── 一阶扩散关系图
├── 节点详情与发现路径
├── 关系证据时间线
├── 消息与空间内容检索
├── 头像/昵称资料时间线
├── AI 分析与审核队列
└── 采集任务与数据源状态
```

## Vuetify MCP

开发阶段可连接官方 MCP：

```json
{
  "mcpServers": {
    "vuetify-mcp": {
      "url": "https://mcp.vuetifyjs.com/mcp"
    }
  }
}
```

如果客户端不支持远程 HTTP MCP，可使用 stdio bridge：

```json
{
  "mcpServers": {
    "vuetify-mcp": {
      "command": "npx",
      "args": ["-y", "mcp-remote", "https://mcp.vuetifyjs.com/mcp"]
    }
  }
}
```

MCP 只提供开发期组件知识，不处理项目数据、不读取数据库，也不进入生产包。使用带 API Key 的配置时，Key 只能放在本机客户端配置或环境变量中，不能提交到仓库。
