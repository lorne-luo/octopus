# OAuth Provider 模型加入分组功能 - 实施计划

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** 在分组编辑页面支持从 OAuth Provider 选择模型，使用负值 channel_id 标识 OAuth Provider。

**Architecture:** 复用现有 `group_items.channel_id` 字段，负值表示 OAuth Provider ID。前端增加 Tab 切换，后端新增 `/channel-list` 端点，路由层识别负值并从 OAuth Provider 获取配置转发。

**Tech Stack:** Go, Gin, GORM, React, Next.js, TanStack Query

---

## Task 1: 数据模型 - 增加 BaseURL 字段

**Files:**
- Modify: `internal/model/oauth_provider.go`
- Modify: `internal/model/oauth_provider.go` (UpdateRequest)

**Step 1: 在 OAuthProvider 结构体增加 BaseURL 字段**

在 `internal/model/oauth_provider.go` 的 `OAuthProvider` 结构体中添加字段：

```go
// 在 CustomModel 字段后添加
BaseURL string `gorm:"size:255" json:"base_url"`
```

**Step 2: 在 OAuthProviderUpdateRequest 增加 BaseURL 字段**

在 `internal/model/oauth_provider.go` 的 `OAuthProviderUpdateRequest` 结构体中添加字段：

```go
// 在 CustomModel 字段后添加
BaseURL *string `json:"base_url,omitempty"`
```

**Step 3: 验证编译通过**

Run: `go build ./...`
Expected: 编译成功，无错误

**Step 4: Commit**

```bash
git add internal/model/oauth_provider.go
git commit -m "feat(model): add base_url field to OAuthProvider"
```

---

## Task 2: 操作层 - 支持 BaseURL 更新和获取方法

**Files:**
- Modify: `internal/op/oauth_provider.go`

**Step 1: 在 OAuthProviderUpdate 函数增加 BaseURL 更新逻辑**

在 `internal/op/oauth_provider.go` 的 `OAuthProviderUpdate` 函数中，在 `if req.CustomModel != nil` 块后添加：

```go
if req.BaseURL != nil {
	updates["base_url"] = *req.BaseURL
}
```

**Step 2: 添加 GetBaseURL 方法**

在 `internal/op/oauth_provider.go` 文件末尾添加：

```go
// GetBaseURL returns the base URL for the OAuth Provider
// Returns configured BaseURL or default based on ProviderType
func (p *model.OAuthProvider) GetBaseURL() string {
	if p.BaseURL != "" {
		return p.BaseURL
	}
	switch p.ProviderType {
	case "iflow":
		return "https://apis.iflow.cn/v1"
	default:
		return ""
	}
}
```

**Step 3: 验证编译通过**

Run: `go build ./...`
Expected: 编译成功，无错误

**Step 4: Commit**

```bash
git add internal/op/oauth_provider.go
git commit -m "feat(op): add base_url update support and GetBaseURL method"
```

---

## Task 3: API 层 - 新增 channel-list 端点

**Files:**
- Modify: `internal/server/handlers/oauth_provider.go`

**Step 1: 在 init() 函数注册新路由**

在 `internal/server/handlers/oauth_provider.go` 的 `init()` 函数中，在 `fetch-model` 路由后添加：

```go
AddRoute(
	router.NewRoute("/channel-list", http.MethodGet).
		Handle(listOAuthProviderChannels),
)
```

**Step 2: 添加 parseModels 辅助函数**

在 `internal/server/handlers/oauth_provider.go` 文件末尾添加：

```go
// parseModels parses comma-separated model strings and returns unique model names
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

**Step 3: 添加 listOAuthProviderChannels 处理函数**

在 `parseModels` 函数前添加：

```go
func listOAuthProviderChannels(c *gin.Context) {
	providers, err := op.OAuthProviderList(c.Request.Context())
	if err != nil {
		resp.Error(c, http.StatusInternalServerError, err.Error())
		return
	}

	var result []model.LLMChannel
	for _, provider := range providers {
		// Only return active providers with API key
		if provider.Status != 1 || provider.APIKey == "" {
			continue
		}

		models := parseModels(provider.Model, provider.CustomModel)
		for _, modelName := range models {
			result = append(result, model.LLMChannel{
				Name:        modelName,
				Enabled:     true,
				ChannelID:   -provider.ID, // Negative ID indicates OAuth Provider
				ChannelName: provider.Name,
			})
		}
	}

	resp.Success(c, result)
}
```

**Step 4: 确保导入 strings 包**

检查文件顶部是否有 `"strings"` 导入，如果没有则添加。

**Step 5: 验证编译通过**

Run: `go build ./...`
Expected: 编译成功，无错误

**Step 6: Commit**

```bash
git add internal/server/handlers/oauth_provider.go
git commit -m "feat(api): add /channel-list endpoint for OAuth Provider models"
```

---

## Task 4: 路由层 - 支持负值 channel_id

**Files:**
- Modify: `internal/relay/relay.go`

**Step 1: 重构 Handler 函数的 channel 获取逻辑**

在 `internal/relay/relay.go` 的 `Handler` 函数中，找到以下代码块（约 line 87-96）：

```go
item := iter.Item()

// 获取通道
channel, err := op.ChannelGet(item.ChannelID, c.Request.Context())
```

替换为：

```go
item := iter.Item()

// 获取通道或 OAuth Provider
var channel *dbmodel.Channel
var oauthProvider *dbmodel.OAuthProvider
var channelType int
var channelName string
var baseUrl string

if item.ChannelID > 0 {
	// 现有 Channel 逻辑
	channel, err = op.ChannelGet(item.ChannelID, c.Request.Context())
	if err != nil {
		log.Warnf("failed to get channel %d: %v", item.ChannelID, err)
		iter.Skip(item.ChannelID, 0, fmt.Sprintf("channel_%d", item.ChannelID), 0, "", fmt.Sprintf("channel not found: %v", err))
		lastErr = err
		continue
	}
	if !channel.Enabled {
		iter.Skip(channel.ID, 0, channel.Name, int(channel.Type), "", "channel disabled")
		continue
	}
	channelType = int(channel.Type)
	channelName = channel.Name
	baseUrl = channel.GetBaseUrl()
} else if item.ChannelID < 0 {
	// OAuth Provider 逻辑
	oauthProvider, err = op.OAuthProviderGet(-item.ChannelID, c.Request.Context())
	if err != nil {
		log.Warnf("failed to get oauth provider %d: %v", -item.ChannelID, err)
		iter.Skip(item.ChannelID, 0, "", 0, "", fmt.Sprintf("oauth provider not found: %v", err))
		lastErr = err
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
} else {
	// channel_id = 0 is invalid
	iter.Skip(item.ChannelID, 0, "", 0, "", "invalid channel_id: 0")
	continue
}
```

**Step 2: 重构 Key 获取逻辑**

找到以下代码块（约 line 102-121）：

```go
var usedKey dbmodel.ChannelKey
if channel.UseOAuth {
	apiKey, err := GetChannelKey(c.Request.Context(), channel)
	...
} else {
	usedKey = channel.GetChannelKey()
}
```

替换为：

```go
var usedKey dbmodel.ChannelKey
if item.ChannelID > 0 {
	if channel.UseOAuth {
		apiKey, err := GetChannelKey(c.Request.Context(), channel)
		if err != nil {
			iter.Skip(channel.ID, 0, channel.Name, int(channel.Type), "", "oauth failed: "+err.Error())
			continue
		}
		usedKey = dbmodel.ChannelKey{
			ChannelID:  channel.ID,
			ChannelKey: apiKey,
			Enabled:    true,
		}
	} else {
		usedKey = channel.GetChannelKey()
	}
} else if item.ChannelID < 0 {
	// OAuth Provider 直接使用其 API Key
	usedKey = dbmodel.ChannelKey{
		ChannelKey: oauthProvider.APIKey,
		Enabled:    true,
	}
}
```

**Step 3: 重构后续使用 channel 的逻辑**

找到 `if usedKey.ChannelKey == ""` 检查后的代码，将 `channel.ID`、`channel.Name`、`channel.Type` 替换为变量：

- `channel.ID` → 使用 `item.ChannelID` 或 `channel.ID`（根据分支）
- `channel.Name` → 使用 `channelName`
- `channel.Type` → 使用 `outbound.OutboundType(channelType)`

具体修改：

1. `iter.SkipCircuitBreak(channel.ID, usedKey.ID, channel.Name, int(channel.Type), apiKeySuffix)` 改为：
   ```go
   if iter.SkipCircuitBreak(item.ChannelID, usedKey.ID, channelName, channelType, apiKeySuffix) {
       continue
   }
   ```

2. `outbound.Get(channel.Type)` 改为：
   ```go
   outAdapter := outbound.Get(outbound.OutboundType(channelType))
   ```

3. 其他 `channel.Type` 相关检查改为使用 `channelType`

**Step 4: 重构 baseUrl 获取**

找到 `channel.GetBaseUrl()` 的使用，确保使用 `baseUrl` 变量。

**Step 5: 验证编译通过**

Run: `go build ./...`
Expected: 编译成功，无错误

**Step 6: Commit**

```bash
git add internal/relay/relay.go
git commit -m "feat(relay): support negative channel_id for OAuth Provider routing"
```

---

## Task 5: 前端 API - 新增 Hook

**Files:**
- Modify: `web/src/api/endpoints/oauthProvider.ts`

**Step 1: 添加 useOAuthProviderChannelList Hook**

在 `web/src/api/endpoints/oauthProvider.ts` 文件末尾添加：

```typescript
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

**Step 2: 确保 LLMChannel 类型已导入**

检查文件顶部是否有 `LLMChannel` 类型导入，如果没有则添加：

```typescript
import { LLMChannel } from './model';
```

**Step 3: 验证 TypeScript 编译**

Run: `cd web && pnpm tsc --noEmit`
Expected: 编译成功，无错误

**Step 4: Commit**

```bash
git add web/src/api/endpoints/oauthProvider.ts
git commit -m "feat(frontend): add useOAuthProviderChannelList hook"
```

---

## Task 6: 前端 UI - Group Editor 增加 Tab

**Files:**
- Modify: `web/src/components/modules/group/Editor.tsx`

**Step 1: 添加 useOAuthProviderChannelList Hook 调用**

在 `GroupEditor` 组件中，找到 `useModelChannelList()` 调用处，添加：

```typescript
const { data: modelChannels = [] } = useModelChannelList();
const { data: oauthProviderChannels = [] } = useOAuthProviderChannelList();  // 新增
```

**Step 2: 修改 ModelPickerSection 组件 Props**

在 `ModelPickerSection` 函数定义处，添加新 prop：

```typescript
function ModelPickerSection({
    modelChannels,
    oauthProviderChannels,  // 新增
    selectedMembers,
    onAdd,
    onAutoAdd,
    autoAddDisabled,
}: {
    modelChannels: LLMChannel[];
    oauthProviderChannels: LLMChannel[];  // 新增
    selectedMembers: SelectedMember[];
    onAdd: (channel: LLMChannel) => void;
    onAutoAdd: () => void;
    autoAddDisabled: boolean;
}) {
```

**Step 3: 在 ModelPickerSection 内添加 Tab 状态和切换 UI**

在 `ModelPickerSection` 函数体开头添加：

```typescript
const [activeTab, setActiveTab] = useState<'channels' | 'oauth'>('channels');
const displayChannels = activeTab === 'channels' ? modelChannels : oauthProviderChannels;
```

**Step 4: 修改 channels 变量使用 displayChannels**

将原来的 `channels` 计算逻辑改为使用 `displayChannels`：

```typescript
const channels = useMemo(() => {
    const byId = new Map<number, { id: number; name: string; models: LLMChannel[] }>();
    displayChannels.forEach((mc) => {  // modelChannels -> displayChannels
        // ... 其余逻辑不变
    });
    // ...
}, [displayChannels]);  // 依赖改为 displayChannels
```

**Step 5: 在 Tab 栏添加切换按钮**

找到 `ModelPickerSection` 的 header 区域（包含 "添加项目" 文字的部分），在标题和自动添加按钮之间添加 Tab 切换：

```tsx
<div className="flex items-center justify-between px-3 py-2 border-b border-border/30 bg-muted/50">
    <div className="flex items-center gap-2">
        <span className="text-sm font-medium text-foreground">
            {t('form.addItem')}
            <span className="ml-1.5 text-xs text-muted-foreground font-normal">
                ({selectedMembers.length})
            </span>
        </span>
    </div>

    <div className="flex items-center gap-2">
        {/* Tab 切换 */}
        <div className="flex rounded-lg bg-muted p-0.5">
            <button
                type="button"
                onClick={() => setActiveTab('channels')}
                className={cn(
                    'px-2 py-0.5 text-xs rounded-md transition-colors',
                    activeTab === 'channels'
                        ? 'bg-background text-foreground shadow-sm'
                        : 'text-muted-foreground hover:text-foreground'
                )}
            >
                {t('form.channels')}
            </button>
            <button
                type="button"
                onClick={() => setActiveTab('oauth')}
                className={cn(
                    'px-2 py-0.5 text-xs rounded-md transition-colors',
                    activeTab === 'oauth'
                        ? 'bg-background text-foreground shadow-sm'
                        : 'text-muted-foreground hover:text-foreground'
                )}
            >
                {t('form.oauthProviders')}
            </button>
        </div>

        {/* 自动添加按钮 */}
        <button
            type="button"
            onClick={onAutoAdd}
            // ... 原有属性
        >
            ...
        </button>
    </div>
</div>
```

**Step 6: 更新 GroupEditor 组件中 ModelPickerSection 的调用**

找到 `<ModelPickerSection` 调用处，添加 `oauthProviderChannels` prop：

```tsx
<ModelPickerSection
    modelChannels={modelChannels}
    oauthProviderChannels={oauthProviderChannels}  // 新增
    selectedMembers={selectedMembers}
    onAdd={handleAddMember}
    onAutoAdd={handleAutoAdd}
    autoAddDisabled={autoAddDisabled}
/>
```

**Step 7: 确保 useState 已导入**

检查文件顶部是否已导入 `useState`，确保包含：

```typescript
import { useCallback, useMemo, useState, type FormEvent } from 'react';
```

**Step 8: 验证 TypeScript 编译**

Run: `cd web && pnpm tsc --noEmit`
Expected: 编译成功，无错误

**Step 9: Commit**

```bash
git add web/src/components/modules/group/Editor.tsx
git commit -m "feat(frontend): add OAuth Provider tab in Group Editor"
```

---

## Task 7: 国际化 - 添加翻译

**Files:**
- Modify: `web/public/locale/en.json`
- Modify: `web/public/locale/zh_hans.json`
- Modify: `web/public/locale/zh_hant.json`

**Step 1: 在 en.json 的 group.form 部分添加翻译**

在 `web/public/locale/en.json` 的 `group.form` 对象中添加：

```json
"channels": "Channels",
"oauthProviders": "OAuth Providers"
```

**Step 2: 在 zh_hans.json 的 group.form 部分添加翻译**

在 `web/public/locale/zh_hans.json` 的 `group.form` 对象中添加：

```json
"channels": "渠道",
"oauthProviders": "OAuth 提供商"
```

**Step 3: 在 zh_hant.json 的 group.form 部分添加翻译**

在 `web/public/locale/zh_hant.json` 的 `group.form` 对象中添加：

```json
"channels": "渠道",
"oauthProviders": "OAuth 提供者"
```

**Step 4: 验证 JSON 格式正确**

Run: `cd web && pnpm tsc --noEmit`
Expected: 编译成功，无错误

**Step 5: Commit**

```bash
git add web/public/locale/en.json web/public/locale/zh_hans.json web/public/locale/zh_hant.json
git commit -m "feat(i18n): add translations for OAuth Provider tab"
```

---

## Task 8: 集成测试

**Step 1: 启动后端服务**

Run: `go run main.go start`
Expected: 服务启动成功，数据库自动迁移

**Step 2: 启动前端开发服务**

Run: `cd web && pnpm dev`
Expected: 前端服务启动成功

**Step 3: 验证功能**

1. 访问分组编辑页面
2. 确认 Tab 切换按钮显示
3. 切换到 "OAuth Providers" Tab
4. 确认 OAuth Provider 模型列表显示
5. 选择模型加入分组
6. 保存分组
7. 验证数据库 `group_items` 表中 `channel_id` 为负值

**Step 4: 最终 Commit**

```bash
git add -A
git commit -m "feat: complete OAuth Provider group integration

- Add base_url field to OAuthProvider model
- Add /channel-list API endpoint
- Support negative channel_id in relay routing
- Add OAuth Provider tab in Group Editor
- Add i18n translations"
```

---

## 总结

| Task | 描述 | 文件数 |
|------|------|--------|
| 1 | 数据模型增加 BaseURL 字段 | 1 |
| 2 | 操作层支持 BaseURL 更新 | 1 |
| 3 | API 层新增 channel-list 端点 | 1 |
| 4 | 路由层支持负值 channel_id | 1 |
| 5 | 前端 API Hook | 1 |
| 6 | 前端 UI Tab 切换 | 1 |
| 7 | 国际化翻译 | 3 |
| 8 | 集成测试 | - |

---

**Document Version**: 1.0
**Created**: 2026-02-15
