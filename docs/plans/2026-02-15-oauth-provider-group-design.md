# OAuth Provider 模型加入分组功能 - 设计文档

## 1. 概述

### 1.1 背景
当前分组（Group）只支持从 Channel 添加模型，用户希望也能从 OAuth Provider 添加模型，实现统一的模型管理。

### 1.2 目标
- 在分组编辑页面支持从 OAuth Provider 选择模型
- OAuth Provider 模型与 Channel 模型在分组中同等对待
- 复用现有 `group_items` 表结构，最小化改动

### 1.3 非目标
- 不修改分组的数据模型结构
- 不支持 OAuth Provider 的 per-key 统计和熔断

## 2. 核心设计决策

### 2.1 channel_id 负值约定

**方案**：使用 `group_items.channel_id` 的负值表示 OAuth Provider

| channel_id | 含义 |
|------------|------|
| `> 0` | 关�� `channels.id` |
| `< 0` | 关联 `oauth_providers.id` (取绝对值) |
| `= 0` | 无效 |

**理由**：
- 复用现有表结构，无需数据库迁移
- 前端逻辑统一，`LLMChannel` 类型可直接复用
- 后端只需增加判断分支

### 2.2 整体架构

```
┌─────────────────────────────────────────────────────────┐
│                    Frontend                              │
│  ┌─────────────────────────────────────────────────┐    │
│  │ Group Editor                                     │    │
│  │  ├─ [Channels] Tab ──────→ Channel 模型列表      │    │
│  │  └─ [OAuth Providers] Tab → OAuth 模型列表       │    │
│  └───────────────────��─────────────────────────────┘    │
└─────────────────────────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────┐
│                    Backend API                           │
│  GET /api/v1/oauth-provider/channel-list                │
│  → 返回 LLMChannel[] 格式，channel_id 为负值             │
└─────────────────────────────────────────────────────────┘
                           │
                           ▼
┌─────────────────────────────────────────────────────────┐
│                    Relay Layer                           │
│  if channel_id > 0 → ChannelGet()                       │
│  if channel_id < 0 → OAuthProviderGet() + 构造转发配置   │
└─────────────────────────────────────────────────────────┘
```

## 3. 数据模型变更

### 3.1 OAuthProvider 表增加字段

```go
// internal/model/oauth_provider.go
type OAuthProvider struct {
    ID               int    `gorm:"primaryKey" json:"id"`
    Name             string `gorm:"size:255;not null" json:"name"`
    ProviderType     string `gorm:"size:50;not null" json:"provider_type"`
    Cookie           string `gorm:"type:text;not null" json:"-"`
    KeyName          string `gorm:"size:255" json:"-"`
    APIKey           string `gorm:"size:255" json:"api_key"`
    APIKeyExpireAt   int64  `json:"api_key_expire_at"`
    Status           int    `gorm:"default:1" json:"status"`
    LastRefreshAt    int64  `json:"last_refresh_at"`
    RefreshFailCount int    `json:"refresh_fail_count"`
    CreatedAt        int64  `json:"created_at"`
    UpdatedAt        int64  `json:"updated_at"`
    Model            string `gorm:"type:text" json:"model"`
    CustomModel      string `gorm:"type:text" json:"custom_model"`
    BaseURL          string `gorm:"size:255" json:"base_url"`  // 新增
}
```

### 3.2 OAuthProviderUpdateRequest 增加字段

```go
type OAuthProviderUpdateRequest struct {
    ID           int     `json:"id" binding:"required"`
    Name         *string `json:"name,omitempty"`
    ProviderType *string `json:"provider_type,omitempty"`
    Cookie       *string `json:"cookie,omitempty"`
    Status       *int    `json:"status,omitempty"`
    Model        *string `json:"model,omitempty"`
    CustomModel  *string `json:"custom_model,omitempty"`
    BaseURL      *string `json:"base_url,omitempty"`  // 新增
}
```

## 4. 后端 API 设计

### 4.1 新增端点：获取 OAuth Provider 模型列表

```
GET /api/v1/oauth-provider/channel-list
Authorization: Bearer {jwt_token}

Response:
[
  {
    "name": "gpt-4",
    "enabled": true,
    "channel_id": -1,
    "channel_name": "IFlow Provider"
  },
  {
    "name": "claude-3-opus",
    "enabled": true,
    "channel_id": -1,
    "channel_name": "IFlow Provider"
  }
]
```

**逻辑**：
- 只返回 `status = 1` 且 `api_key != ""` 的 Provider
- 解析 `model` + `custom_model` 字段（逗号分隔）
- `channel_id` 取负值（`-provider.ID`）

### 4.2 实现

```go
// internal/server/handlers/oauth_provider.go

func listOAuthProviderChannels(c *gin.Context) {
    providers, err := op.OAuthProviderList(c.Request.Context())
    if err != nil {
        resp.Error(c, http.StatusInternalServerError, err.Error())
        return
    }

    var result []model.LLMChannel
    for _, provider := range providers {
        if provider.Status != 1 || provider.APIKey == "" {
            continue
        }

        models := parseModels(provider.Model, provider.CustomModel)
        for _, modelName := range models {
            result = append(result, model.LLMChannel{
                Name:        modelName,
                Enabled:     true,
                ChannelID:   -provider.ID,
                ChannelName: provider.Name,
            })
        }
    }

    resp.Success(c, result)
}

func parseModels(model, customModel string) []string {
    seen := make(map[string]struct{})
    var result []string

    for _, m := range strings.Split(model, ",") {
        m = strings.TrimSpace(m)
        if m != "" {
            if _, exists := seen[m]; !exists {
                seen[m] = struct{}{}
                result = append(result, m)
            }
        }
    }

    for _, m := range strings.Split(customModel, ",") {
        m = strings.TrimSpace(m)
        if m != "" {
            if _, exists := seen[m]; !exists {
                seen[m] = struct{}{}
                result = append(result, m)
            }
        }
    }

    return result
}
```

## 5. 后端路由逻辑变更

### 5.1 relay.go 核心变更

**变更位置**：`internal/relay/relay.go` 的 `Handler` 函数

```go
// 原逻辑 (line 89-96)
channel, err := op.ChannelGet(item.ChannelID, c.Request.Context())

// 新逻辑
var channel *dbmodel.Channel
var oauthProvider *dbmodel.OAuthProvider
var channelType int
var channelName string
var baseUrl string

if item.ChannelID > 0 {
    // 现有 Channel 逻辑
    channel, err = op.ChannelGet(item.ChannelID, c.Request.Context())
    if err != nil {
        iter.Skip(item.ChannelID, 0, "", 0, "", "channel not found")
        continue
    }
    channelType = int(channel.Type)
    channelName = channel.Name
    baseUrl = channel.GetBaseUrl()
} else if item.ChannelID < 0 {
    // OAuth Provider 逻辑
    oauthProvider, err = op.OAuthProviderGet(-item.ChannelID, c.Request.Context())
    if err != nil {
        iter.Skip(item.ChannelID, 0, "", 0, "", "oauth provider not found")
        continue
    }
    if oauthProvider.Status != 1 {
        iter.Skip(item.ChannelID, 0, oauthProvider.Name, 0, "", "oauth provider disabled")
        continue
    }
    if oauthProvider.APIKey == "" {
        iter.Skip(item.ChannelID, 0, oauthProvider.Name, 0, "", "oauth provider has no api key")
        continue
    }
    channelType = int(outbound.OutboundTypeOpenAIChat)
    channelName = oauthProvider.Name
    baseUrl = oauthProvider.GetBaseURL()
}
```

### 5.2 Key 获取逻辑

```go
var usedKey dbmodel.ChannelKey

if item.ChannelID > 0 {
    if channel.UseOAuth {
        apiKey, err := GetChannelKey(c.Request.Context(), channel)
        if err != nil {
            iter.Skip(channel.ID, 0, channel.Name, int(channel.Type), "", "oauth failed: "+err.Error())
            continue
        }
        usedKey = dbmodel.ChannelKey{ChannelKey: apiKey, Enabled: true}
    } else {
        usedKey = channel.GetChannelKey()
    }
} else if item.ChannelID < 0 {
    // OAuth Provider 直接使用其 API Key
    usedKey = dbmodel.ChannelKey{ChannelKey: oauthProvider.APIKey, Enabled: true}
}
```

### 5.3 统计与熔断逻辑

```go
// OAuth Provider 使用负值 ID 作为统一标识
if item.ChannelID > 0 {
    op.StatsChannelUpdate(channel.ID, dbmodel.StatsMetrics{...})
    balancer.RecordSuccess(channel.ID, usedKey.ID, model)
} else if item.ChannelID < 0 {
    op.StatsChannelUpdate(item.ChannelID, dbmodel.StatsMetrics{...})
    balancer.RecordSuccess(item.ChannelID, 0, model)  // keyID = 0
}
```

### 5.4 OAuth Provider 操作层扩展

```go
// internal/op/oauth_provider.go

func (p *OAuthProvider) GetBaseURL() string {
    if p.BaseURL != "" {
        return p.BaseURL
    }
    // 根据 ProviderType 返回默认 URL
    switch p.ProviderType {
    case "iflow":
        return "https://apis.iflow.cn/v1"
    default:
        return ""
    }
}
```

## 6. 前端设计

### 6.1 API Hook

```typescript
// web/src/api/endpoints/oauthProvider.ts

export function useOAuthProviderChannelList() {
    return useQuery({
        queryKey: ['oauth-provider', 'channel-list'],
        queryFn: async () => {
            return apiClient.get<LLMChannel[]>('/api/v1/oauth-provider/channel-list');
        },
        refetchInterval: 30000,
    });
}
```

### 6.2 Group Editor 组件变更

**ModelPickerSection 增加 Tab 切换**

```tsx
function ModelPickerSection({
    modelChannels,
    oauthProviderChannels,  // 新增
    selectedMembers,
    onAdd,
    onAutoAdd,
    autoAddDisabled,
}: {
    modelChannels: LLMChannel[];
    oauthProviderChannels: LLMChannel[];
    selectedMembers: SelectedMember[];
    onAdd: (channel: LLMChannel) => void;
    onAutoAdd: () => void;
    autoAddDisabled: boolean;
}) {
    const [activeTab, setActiveTab] = useState<'channels' | 'oauth'>('channels');
    const displayChannels = activeTab === 'channels' ? modelChannels : oauthProviderChannels;
    // ... 渲染逻辑
}
```

### 6.3 UI 布局

```
┌─────────────────────────────────────────────┐
│ 添加项目 (3)                       [自动添加] │
├─────────────────────────────────────────────┤
│ [Channels] [OAuth Providers]                 │  ← Tab 切换
├─────────────────────────────────────────────┤
│ ▼ Channel A                          5/12   │
│   ┌─────────────────────────────────────┐   │
│   │ gpt-4                           [+] │   │
│   │ gpt-3.5-turbo                  [+] │   │
│   └─────────────────────────────────────┘   │
│ ▼ OAuth: IFlow Provider              0/8    │
│   ┌─────────────────────────────────────┐   │
│   │ claude-3-opus                  [+] │   │
│   │ claude-3-sonnet                [+] │   │
│   └─────────────────────────────────────┘   │
└─────────────────────────────────────────────┘
```

## 7. 实施计划

### 7.1 变更文件清单

| 序号 | 文件 | 改动类型 | 说明 |
|------|------|----------|------|
| 1 | `internal/model/oauth_provider.go` | 修改 | 增加 `BaseURL` 字段 |
| 2 | `internal/op/oauth_provider.go` | 修改 | 增加 `GetBaseURL()` 方法，更新 Update 支持 base_url |
| 3 | `internal/server/handlers/oauth_provider.go` | 修改 | 新增 `/channel-list` 端点 |
| 4 | `internal/relay/relay.go` | 修改 | 支持负值 channel_id 路由 |
| 5 | `web/src/api/endpoints/oauthProvider.ts` | 修改 | 新增 `useOAuthProviderChannelList` hook |
| 6 | `web/src/components/modules/group/Editor.tsx` | 修改 | 增加 OAuth Provider Tab |
| 7 | `web/public/locale/en.json` | 修改 | 新增 i18n 翻译 |
| 8 | `web/public/locale/zh.json` | 修改 | 新增 i18n 翻译 |

### 7.2 实施顺序

```
Phase 1: 数据模型 (后端)
├── 1.1 OAuthProvider 表增加 base_url 字段
└── 1.2 数据库自动迁移

Phase 2: 后端 API (后端)
├── 2.1 新增 /channel-list 端点
└── 2.2 OAuthProviderUpdate 支持 base_url

Phase 3: 路由逻辑 (后端)
└── 3.1 relay.go 支持负值 channel_id

Phase 4: 前端 (前端)
├── 4.1 新增 API hook
├── 4.2 Editor.tsx 增加 Tab
└── 4.3 i18n 翻译

Phase 5: 测试验证
├── 5.1 后端单元测试
└── 5.2 集成测试
```

## 8. 风险与应对

| 风险 | 影响 | 应对措施 |
|------|------|----------|
| 负值 channel_id 与现有逻辑冲突 | 中 | 在所有 `ChannelGet` 调用处增加判断 |
| OAuth Provider API Key 过期 | 低 | 路由层已设计 Status 和 APIKey 检查 |
| 熔断器不支持负值 ID | 低 | 熔断器使用 int 类型，负值正常工作 |

## 9. 未来扩展

- 支持更多 ProviderType 及其默认 base_url
- OAuth Provider 维度的统计面板
- OAuth Provider 批量刷新功能

---

**Document Version**: 1.0
**Created**: 2026-02-15
**Status**: Approved for Implementation
