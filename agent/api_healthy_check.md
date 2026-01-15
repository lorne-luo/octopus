# Channel Health Check 功能开发计划

## 需求概述

添加一个 Health 模块，用于定期检测 Channel 的健康状态。

### 用户界面需求

1. 在顶级菜单添加一个 Health 菜单
2. 点进 Health 后，是 channel 健康检测项的列表页，风格与其他菜单比如 channel group 的类似
3. 用户可以点击 + 按钮，添加针对 channel 的健康检测项
4. 在添加检测项的窗口中，用户可以指定：
   - channel
   - model name（如 gpt-5）
   - 频率（每小时/每天）
   - 提示词（如 hi）
   - 最后保存
5. 健康检测项将被注册进 `internal/task/task.go`，后台会按照指定频率发送提示词到 channel 的 model
6. 如果访问成功，在健康检测项的列表页卡片上显示绿色；如果不成功则显示红色

---

## 架构设计

### 数据流

```
┌─────────────────────────────────────────────────────────────────────┐
│                         前端 (Web)                                  │
├─────────────────────────────────────────────────────────────────────┤
│  Health 菜单 -> 列表页(卡片网格) -> 添加/编辑对话框                      │
│         ↓                                                          │
│  API 调用 (GET/POST/PUT/DELETE /api/v1/health/*)                    │
└─────────────────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────────────┐
│                      后端 API Layer                                 │
├─────────────────────────────────────────────────────────────────────┤
│  internal/server/handlers/health.go                                  │
│    - list, create, update, delete, toggle handlers                  │
└─────────────────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────────────┐
│                      Business Layer                                 │
├─────────────────────────────────────────────────────────────────────┤
│  internal/op/health.go                                                │
│    - CRUD operations with cache                                      │
└─────────────────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────────────┐
│                      Data Layer                                     │
├─────────────────────────────────────────────────────────────────────┤
│  internal/model/health.go                                             │
│    - HealthCheck struct + GORM definitions                          │
│         ↓                                                          │
│  SQLite/MySQL/PostgreSQL Table: health_checks                        │
└─────────────────────────────────────────────────────────────────────┘
                              ↓
┌─────────────────────────────────────────────────────────────────────┐
│                    健康检查执行层                                     │
├─────────────────────────────────────────────────────────────────────┤
│  internal/task/health.go                                              │
│    - RunHealthCheck(id) - 发送请求到 channel，更新最后检查时间和状态   │
│         ↓                                                          │
│  internal/task/init.go                                               │
│    - RegisterHealthTasks() - 每个 HealthCheck 项注册为独立任务       │
│         ↓                                                          │
│  internal/task/task.go                                               │
│    - Register/Update 调度系统 (已存在)                               │
└─────────────────────────────────────────────────────────────────────┘
```

---

## 开发任务清单

### 1. 后端 - 数据模型 (`internal/model/health.go`)

```go
type HealthCheckStatus string

const (
    HealthCheckStatusUnknown   HealthCheckStatus = "unknown"   // 未检查
    HealthCheckStatusHealthy   HealthCheckStatus = "healthy"   // 健康绿色
    HealthCheckStatusUnhealthy HealthCheckStatus = "unhealthy" // 不健康红色
    HealthCheckStatusChecking  HealthCheckStatus = "checking"  // 检查中
)

// 检查频率类型
type CheckInterval string

const (
    CheckIntervalHourly CheckInterval = "hourly" // 每小时
    CheckIntervalDaily  CheckInterval = "daily"  // 每天
)

type HealthCheck struct {
    ID          int                `json:"id" gorm:"primaryKey"`
    ChannelID   int                `json:"channel_id" gorm:"not null;index:idx_channel_index,unique"`
    ModelName   string             `json:"model_name" gorm:"not null;index:idx_channel_index,unique"`
    Interval    CheckInterval      `json:"interval" gorm:"not null"`     // hourly/daily
    Prompt      string             `json:"prompt" gorm:"not null"`
    Status      HealthCheckStatus  `json:"status" gorm:"default:'unknown'"`
    LastCheck   *time.Time         `json:"last_check"`
    NextCheck   *time.Time         `json:"next_check"`
    LastError   *string            `json:"last_error"`
    CreatedAt   time.Time          `json:"created_at"`
    UpdatedAt   time.Time          `json:"updated_at"`
}

// 关联查询用
type HealthCheckWithChannel struct {
    HealthCheck
    ChannelName string `json:"channel_name"`
}
```

### 2. 后端 - 业务逻辑 (`internal/op/health.go`)

实现 CRUD 操作，参考 `internal/op/channel.go` 的模式：
- `HealthList(ctx) ([]HealthCheckWithChannel, error)` - 获取列表（带 channel 名称）
- `HealthCreate(ctx, HealthCheck) error` - 创建（检查 channel/model 组合唯一）
- `HealthUpdate(ctx, HealthCheck) error` - 更新
- `HealthDelete(ctx, id) error` - 删除
- `HealthGet(ctx, id) (*HealthCheck, error)` - 获取单个
- `HealthUpdateStatus(ctx, id, status, error) error` - 更新状态（供 task 调用）

注意：添加/更新时需要调用 `task.RegisterHealthTask(id)` 或 `task.UpdateHealthTask(id)`

### 3. 后端 - API Handlers (`internal/server/handlers/health.go`)

路由定义：
```
GET    /api/v1/health/list       - 列表
POST   /api/v1/health/create     - 创建
POST   /api/v1/health/update     - 更新
DELETE /api/v1/health/delete/:id - 删除
```

需要在 `internal/server/router/` 中注册路由。

### 4. 后端 - 健康检查执行 (`internal/task/health.go`)

核心函数：
```go
// 执行单个健康检查
func RunHealthCheck(id int) {
    // 1. 获取 HealthCheck 和对应的 Channel
    // 2. 构造 LLM 请求（使用 prompt, model, channel）
    // 3. 发送请求
    // 4. 更新 HealthCheck 状态（success -> healthy, error -> unhealthy）
    // 5. 更新 last_check, last_error, next_check
}

// 在 internal/task/init.go 调用：为每个 HealthCheck 项注册任务
func RegisterHealthTask(hc *model.HealthCheck) {
    interval := 1 * time.Hour // hourly
    if hc.Interval == model.CheckIntervalDaily {
        interval = 24 * time.Hour
    }
    task.Register(fmt.Sprintf("health_check_%d", hc.ID), interval, true,
                  func() { RunHealthCheck(hc.ID) })
}
```

### 5. 后端 - 任务注册 (`internal/task/init.go`)

在 `Init()` 函数中添加：
```go
// 加载所有已存在的健康检查并注册任务
healthChecks, err := op.HealthList(context.Background())
if err == nil {
    for _, hc := range healthChecks {
        RegisterHealthTask(&hc)
    }
}
```

### 6. 前端 - API 客户端 (`web/src/api/endpoints/health.ts`)

```typescript
export interface HealthCheck {
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

export interface HealthCheckWithChannel extends HealthCheck {
    channel_name: string;
}

export interface HealthCheckCreateRequest {
    channel_id: number;
    model_name: string;
    interval: 'hourly' | 'daily';
    prompt: string;
}

export interface HealthCheckUpdateRequest {
    id: number;
    model_name?: string;
    interval?: 'hourly' | 'daily';
    prompt?: string;
}

// 使用 TanStack Query 封装 API 调用
export const useHealthList = () => {
    return useQuery({
        queryKey: ['health', 'list'],
        queryFn: () => apiClient.get<ApiResponse<HealthCheckWithChannel[]>>('/api/v1/health/list'),
    });
};

export const useHealthCreate = () => {
    const queryClient = useQueryClient();
    return useMutation({
        mutationFn: (data: HealthCheckCreateRequest) =>
            apiClient.post<ApiResponse>('/api/v1/health/create', data),
        onSuccess: () => {
            queryClient.invalidateQueries({ queryKey: ['health', 'list'] });
        },
    });
};

export const useHealthUpdate = () => {
    const queryClient = useQueryClient();
    return useMutation({
        mutationFn: (data: HealthCheckUpdateRequest) =>
            apiClient.post<ApiResponse>('/api/v1/health/update', data),
        onSuccess: () => {
            queryClient.invalidateQueries({ queryKey: ['health', 'list'] });
        },
    });
};

export const useHealthDelete = () => {
    const queryClient = useQueryClient();
    return useMutation({
        mutationFn: (id: number) =>
            apiClient.delete<ApiResponse>(`/api/v1/health/delete/${id}`),
        onSuccess: () => {
            queryClient.invalidateQueries({ queryKey: ['health', 'list'] });
        },
    });
};
```

### 7. 前端 - 路由配置 (`web/src/route/config.tsx`)

```typescript
import { Activity } from 'lucide-react';  // 或 HeartPulse, Heart

const Health_Module = lazyWithPreload(() => import('@/components/modules/health').then(m => ({ default: m.Health })));

export const ROUTES: RouteConfig[] = [
    // ... existing routes
    {
        id: 'health',
        label: 'Health',
        icon: Activity,
        component: Health_Module,
    },
];
```

### 8. 前端 - 组件 (`web/src/components/modules/health/`)

组件结构 (参考 `web/src/components/modules/channel/`)：
```
health/
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
    &model.HealthCheck{},  // 新增
)
```

---

## 实现文件清单

| 路径 | 说明 |
|------|------|
| `internal/model/health.go` | HealthCheck 数据模型 |
| `internal/op/health.go` | CRUD 业务逻辑 |
| `internal/server/handlers/health.go` | HTTP 处理器 |
| `internal/server/router/health/` (新建) | 路由注册 |
| `internal/task/health.go` | 健康检查执行逻辑 |
| `internal/db/db.go` (修改) | 添加 AutoMigrate |
| `internal/task/init.go` (修改) | 注册健康检查任务 |
| `web/src/api/endpoints/health.ts` | 前端 API 客户端 |
| `web/src/route/config.tsx` (修改) | 添加 Health 路由 |
| `web/src/components/modules/health/index.tsx` | 列表页 |
| `web/src/components/modules/health/Card.tsx` | 卡片组件 |
| `web/src/components/modules/health/CardContent.tsx` | 卡片内容 |
| `web/src/components/modules/health/CreateDialog.tsx` | 创建对话框 |
| `web/src/components/modules/health/Form.tsx` | 表单组件 |

---

## 开发顺序建议

1. **后端数据层**：先创建 Model 和 DB migration
2. **后端业务层**：实现 CRUD 操作
3. **后端执行层**：实现健康检查逻辑和任务注册
4. **后端 API 层**：添加 API handlers 和路由
5. **前端 API 层**：创建 API 客户端
6. **前端 UI 层**：创建列表页和对话框
7. **前端路由**：添加路由配置
8. **集成测试**：端到端测试完整流程