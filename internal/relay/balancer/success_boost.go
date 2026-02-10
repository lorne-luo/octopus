package balancer

import (
	"github.com/bestruirui/octopus/internal/model"
)

// SuccessBoost 成功提权：按优先级排序（同 Failover），但成功后会触发提权逻辑
type SuccessBoost struct {
	Failover
}

func (b *SuccessBoost) Candidates(items []model.GroupItem) []model.GroupItem {
	return b.Failover.Candidates(items)
}
