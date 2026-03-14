# Channel Health Check 功能开发计划

## 需求概述

添加一个 Channel Health 模块，用于定期监测 Channel 的健康状态。

### 用户界面需求

1. 在顶级菜单添加一个 Health 菜单,图标使用Activity
2. 点进 Health 后，是 channel 健康监测项的列表页，风格与其他菜单比如 channel group 的类似
3. 用户可以点击 + 按钮，添加针对 channel 的健康监测项
4. 在添加监测项(channel_health)的窗口中，用户可以指定：
   - channel
   - model name（可选择该channel的一个model如 gpt-5）
   - base url(默认选择第一个)
   - 频率（以分钟为单位的数字input，可以自行输入，但下方有每小时/每4小时/每8小时/每天/每周的快捷选项）
   - 提示词（如 hi）
   - 最后保存
5. 健康监测项将被注册进 `internal/task/task.go`，后台会按照指定频率发送提示词到 channel 的 model
6. 如果访问成功，在健康监测项的列表页卡片上显示绿色icon；如果不成功则显示红色
7. 对于disabled的channel,仍可以创建健康监测，并会被注册执行

---

## 架构设计

### 数据流

```
┌─────────────────────────────────────────────────────────────────────┐
│                         前端 (Web)                                  │
├─────────────────────────────────────────────────────────────────────┤
│  Health 菜单 -> 列表页(卡片网格) -> 添加/编辑对话框                      │
│         ↓                                                          │
│  API 调用 (GET/POST/PUT/DELETE /api/v1/channel_health/*)            │
└─────────────────────────────────────────────────────────────────────┘
                               ↓
┌─────────────────────────────────────────────────────────────────────┐
│                      后端 API Layer                                 │
├─────────────────────────────────────────────────────────────────────┤
│  internal/server/handlers/channel_health.go                         │
│    - list, create, update, delete, toggle handlers                  │
└─────────────────────────────────────────────────────────────────────┘
                               ↓
┌─────────────────────────────────────────────────────────────────────┐
│                      Business Layer                                 │
├─────────────────────────────────────────────────────────────────────┤
│  internal/op/channel_health.go                                      │
│    - CRUD operations with cache                                      │
└─────────────────────────────────────────────────────────────────────┘
                               ↓
┌─────────────────────────────────────────────────────────────────────┐
│                      Data Layer                                     │
├─────────────────────────────────────────────────────────────────────┤
│  internal/model/channel_health.go                                   │
│    - ChannelHealth struct + GORM definitions                        │
│         ↓                                                          │
│  SQLite/MySQL/PostgreSQL Table: channel_health                      │
└─────────────────────────────────────────────────────────────────────┘
                               ↓
┌─────────────────────────────────────────────────────────────────────┐
│                    健康检查执行层                                     │
├─────────────────────────────────────────────────────────────────────┤
│  internal/task/channel_health.go                                    │
│    - RunChannelHealth(id) - 发送请求到 channel，更新最后检查时间和状态   │
│         ↓                                                          │
│  internal/task/init.go                                               │
│    - RegisterChannelHealthTasks() - 每个 ChannelHealth 项注册为独立任务   │
│         ↓                                                          │
│  internal/task/task.go                                               │
│    - Register/Update 调度系统 (已存在)                               │
└─────────────────────────────────────────────────────────────────────┘
```

---

## 开发任务清单

### 1. 后端 - 数据模型 (`internal/model/channel_health.go`)

```go
type ChannelHealthStatus string

const (
    ChannelHealthStatusUnknown   ChannelHealthStatus = "unknown"   // 未检查
    ChannelHealthStatusHealthy   ChannelHealthStatus = "healthy"   // 健康绿色
    ChannelHealthStatusUnhealthy ChannelHealthStatus = "unhealthy" // 不健康红色
    ChannelHealthStatusChecking  ChannelHealthStatus = "checking"  // 检查中
)

// 检查频率类型
type CheckInterval string

const (
    CheckIntervalHourly CheckInterval = "hourly" // 每小时
    CheckIntervalDaily  CheckInterval = "daily"  // 每天
)

type ChannelHealth struct {
    ID          int                 `json:"id" gorm:"primaryKey"`
    ChannelID   int                 `json:"channel_id" gorm:"not null;index:idx_channel_index,unique"`
    ModelName   string              `json:"model_name" gorm:"not null;index:idx_channel_index,unique"`
    Interval    CheckInterval       `json:"interval" gorm:"not null"`     // hourly/daily
    Prompt      string              `json:"prompt" gorm:"not null"`
    Status      ChannelHealthStatus `json:"status" gorm:"default:'unknown'"`
    LastCheck   *time.Time          `json:"last_check"`
    NextCheck   *time.Time          `json:"next_check"`
    LastError   *string             `json:"last_error"`
    CreatedAt   time.Time           `json:"created_at"`
    UpdatedAt   time.Time           `json:"updated_at"`
}

// 关联查询用
type ChannelHealthWithChannel struct {
    ChannelHealth
    ChannelName string `json:"channel_name"`
}
```

### 2. 后端 - 业务逻辑 (`internal/op/channel_health.go`)

实现 CRUD 操作，参考 `internal/op/channel.go` 的模式：
- `ChannelHealthList(ctx) ([]ChannelHealthWithChannel, error)` - 获取列表（带 channel 名称）
- `ChannelHealthCreate(ctx, ChannelHealth) error` - 创建（检查 channel/model 组合唯一）
- `ChannelHealthUpdate(ctx, ChannelHealth) error` - 更新
- `ChannelHealthDelete(ctx, id) error` - 删除
- `ChannelHealthGet(ctx, id) (*ChannelHealth, error)` - 获取单个
- `ChannelHealthUpdateStatus(ctx, id, status, error) error` - 更新状态（供 task 调用）

注意：添加/更新时需要调用 `task.RegisterChannelHealthTask(id)` 或 `task.UpdateChannelHealthTask(id)`

### 3. 后端 - API Handlers (`internal/server/handlers/channel_health.go`)

路由定义：
```
GET    /api/v1/channel_health/list       - 列表
POST   /api/v1/channel_health/create     - 创建
POST   /api/v1/channel_health/update     - 更新
DELETE /api/v1/channel_health/delete/:id - 删除
```

需要在 `internal/server/router/` 中注册路由。

### 4. 后端 - 健康检查执行 (`internal/task/channel_health.go`)

核心函数：
```go
// 执行单个健康检查
func RunChannelHealth(id int) {
    // 1. 获取 ChannelHealth 和对应的 Channel
    // 2. 构造 LLM 请求（使用 prompt, model, channel）
    // 3. 发送请求
    // 4. 更新 ChannelHealth 状态（success -> healthy, error -> unhealthy）
    // 5. 更新 last_check, last_error, next_check
    // 6. 记录日志到 Log 表（使用 op.CreateLog），确保可从日志界面查看
}

// 在 internal/task/init.go 调用：为每个 ChannelHealth 项注册任务
func RegisterChannelHealthTask(hc *model.ChannelHealth) {
    interval := 1 * time.Hour // hourly
    if hc.Interval == model.CheckIntervalDaily {
        interval = 24 * time.Hour
    }
    task.Register(fmt.Sprintf("channel_health_%d", hc.ID), interval, true,
                  func() { RunChannelHealth(hc.ID) })
}
```

#### 日志记录要求：
- 每次健康监测执行后，无论成功或失败，都需要记录到 `Log` 表
- 日志类型可使用 `health_check` 或复用现有类型
- 日志内容需包含：channel_id, model_name, status, error_message (如有)
- 确保日志可在前端日志界面 (`/log`) 中查看和筛选

### 5. 后端 - 任务注册 (`internal/task/init.go`)

在 `Init()` 函数中添加：
```go
// 加载所有已存在的健康检查并注册任务
channelHealths, err := op.ChannelHealthList(context.Background())
if err == nil {
    for _, hc := range channelHealths {
        RegisterChannelHealthTask(&hc)
    }
}
```

### 6. 前端 - API 客户端 (`web/src/api/endpoints/channel_health.ts`)

```typescript
export interface ChannelHealth {
    id: number;
    channel_id: number;
    model_name: string;
    interval: 'hourly' | 'daily';
    prompt: string;
    status: 'unknown' | 'healthy' | 'unhealthy' | 'checking';
    last_check?: string;
    next_check?: string;
    last_error?: string;
    created_at: string;
    updated_at: string;
}

export interface ChannelHealthWithChannel extends ChannelHealth {
    channel_name: string;
}

export interface ChannelHealthCreateRequest {
    channel_id: number;
    model_name: string;
    interval: 'hourly' | 'daily';
    prompt: string;
}

export interface ChannelHealthUpdateRequest {
    id: number;
    model_name?: string;
    interval?: 'hourly' | 'daily';
    prompt?: string;
}

// 使用 TanStack Query 封装 API 调用
export const useChannelHealthList = () => {
    return useQuery({
        queryKey: ['channel_health', 'list'],
        queryFn: () => apiClient.get<ApiResponse<ChannelHealthWithChannel[]>>('/api/v1/channel_health/list'),
    });
};

export const useChannelHealthCreate = () => {
    const queryClient = useQueryClient();
    return useMutation({
        mutationFn: (data: ChannelHealthCreateRequest) =>
            apiClient.post<ApiResponse>('/api/v1/channel_health/create', data),
        onSuccess: () => {
            queryClient.invalidateQueries({ queryKey: ['channel_health', 'list'] });
        },
    });
};

export const useChannelHealthUpdate = () => {
    const queryClient = useQueryClient();
    return useMutation({
        mutationFn: (data: ChannelHealthUpdateRequest) =>
            apiClient.post<ApiResponse>('/api/v1/channel_health/update', data),
        onSuccess: () => {
            queryClient.invalidateQueries({ queryKey: ['channel_health', 'list'] });
        },
    });
};

export const useChannelHealthDelete = () => {
    const queryClient = useQueryClient();
    return useMutation({
        mutationFn: (id: number) =>
            apiClient.delete<ApiResponse>(`/api/v1/channel_health/delete/${id}`),
        onSuccess: () => {
            queryClient.invalidateQueries({ queryKey: ['channel_health', 'list'] });
        },
    });
};
```

### 7. 前端 - 路由配置 (`web/src/route/config.tsx`)

```typescript
import { Activity } from 'lucide-react';  // 或 HeartPulse, Heart

const Channel_Health_Module = lazyWithPreload(() => import('@/components/modules/channel_health').then(m => ({ default: m.ChannelHealth })));

export const ROUTES: RouteConfig[] = [
    // ... existing routes
    {
        id: 'channel_health',
        label: 'Health',
        icon: Activity,
        component: Channel_Health_Module,
    },
];
```

### 8. 前端 - 组件 (`web/src/components/modules/channel_health/`)

组件结构 (参考 `web/src/components/modules/channel/`)：
```
channel_health/
├── index.tsx          # 主列表页
├── Card.tsx           # 健康检查卡片（绿色/红色状态指示）
├── CardContent.tsx    # 卡片内容
├── CreateDialog.tsx   # 添加对话框
└── Form.tsx           # 表单
```

#### Card.tsx 状态指示设计：
- 绿色圆点 / 绿色边框 → Healthy
- 红色圆点 / 红色边框 → Unhealthy
- 灰色圆点 → Unknown
- 闪烁动画 → Checking

#### 错误信息显示：
- 当 `status === 'unhealthy'` 且存在 `last_error` 时，在卡片中显示错误信息
- 错误信息可使用红色文字或浅红色背景展示
- 可考虑截断过长的错误信息，hover 时显示完整内容（使用 Tooltip）

#### Form.tsx 表单字段：
```tsx
<form>
  <Select label="Channel" options={channelList} required />
  <Input label="Model Name" placeholder="gpt-5" required />
  <Select label="Interval" options={[
    { value: 'hourly', label: '每小时' },
    { value: 'daily', label: '每天' },
  ]} required />
  <Textarea label="Prompt" placeholder="hi" required />
</form>
```

### 9. 数据库迁移

在 `internal/db/db.go` 的 `AutoMigrate` 中添加：
```go
db.AutoMigrate(
    &model.User{},
    &model.Channel{},
    &model.ChannelKey{},
    // ... existing models
    &model.ChannelHealth{},  // 新增
)
```

---

## 实现文件清单

| 路径 | 说明 |
|------|------|
| `internal/model/channel_health.go` | ChannelHealth 数据模型 |
| `internal/op/channel_health.go` | CRUD 业务逻辑 |
| `internal/server/handlers/channel_health.go` | HTTP 处理器 |
| `internal/server/router/channel_health/` (新建) | 路由注册 |
| `internal/task/channel_health.go` | 健康检查执行逻辑 |
| `internal/db/db.go` (修改) | 添加 AutoMigrate |
| `internal/task/init.go` (修改) | 注册健康检查任务 |
| `web/src/api/endpoints/channel_health.ts` | 前端 API 客户端 |
| `web/src/route/config.tsx` (修改) | 添加 Health 路由 |
| `web/src/components/modules/channel_health/index.tsx` | 列表页 |
| `web/src/components/modules/channel_health/Card.tsx` | 卡片组件 |
| `web/src/components/modules/channel_health/CardContent.tsx` | 卡片内容 |
| `web/src/components/modules/channel_health/CreateDialog.tsx` | 创建对话框 |
| `web/src/components/modules/channel_health/Form.tsx` | 表单组件 |

---

## 开发顺序建议

1. **后端数据层**：先创建 Model 和 DB migration
2. **后端业务层**：实现 CRUD 操作
3. **后端执行层**：实现健康检查逻辑和任务注册
4. **后端 API 层**：添加 API handlers 和路由
5. **前端 API 层**：创建 API 客户端
6. **前端 UI 层**：创建列表页和对话框
7. **前端 路由**：添加 路由配置
8. **集成测试**：端到端测试完整流程