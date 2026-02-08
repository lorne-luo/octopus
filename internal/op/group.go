package op

import (
	"context"
	"fmt"
	"sort"

	"github.com/bestruirui/octopus/internal/db"
	"github.com/bestruirui/octopus/internal/model"
	"github.com/bestruirui/octopus/internal/utils/cache"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var groupCache = cache.New[int, model.Group](16)
var groupMap = cache.New[string, model.Group](16)

func GroupList(ctx context.Context) ([]model.Group, error) {
	groups := make([]model.Group, 0, groupCache.Len())
	for _, group := range groupCache.GetAll() {
		groups = append(groups, group)
	}
	return groups, nil
}

func GroupListModel(ctx context.Context) ([]string, error) {
	models := []string{}
	for _, group := range groupCache.GetAll() {
		models = append(models, group.Name)
	}
	return models, nil
}

func GroupGet(id int, ctx context.Context) (*model.Group, error) {
	group, ok := groupCache.Get(id)
	if !ok {
		return nil, fmt.Errorf("group not found")
	}
	return &group, nil
}

func GroupGetMap(name string, ctx context.Context) (model.Group, error) {
	items, ok := groupMap.Get(name)
	if !ok {
		return model.Group{}, fmt.Errorf("group not found")
	}
	return items, nil
}

func GroupCreate(group *model.Group, ctx context.Context) error {
	if err := db.GetDB().WithContext(ctx).Create(group).Error; err != nil {
		return err
	}
	groupCache.Set(group.ID, *group)
	groupMap.Set(group.Name, *group)
	return nil
}

func GroupUpdate(req *model.GroupUpdateRequest, ctx context.Context) (*model.Group, error) {
	oldGroup, ok := groupCache.Get(req.ID)
	if !ok {
		return nil, fmt.Errorf("group not found")
	}
	oldName := oldGroup.Name

	tx := db.GetDB().WithContext(ctx).Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	var selectFields []string
	updates := model.Group{ID: req.ID}

	if req.Name != nil {
		selectFields = append(selectFields, "name")
		updates.Name = *req.Name
	}
	if req.Mode != nil {
		selectFields = append(selectFields, "mode")
		updates.Mode = *req.Mode
	}
	if req.MatchRegex != nil {
		selectFields = append(selectFields, "match_regex")
		updates.MatchRegex = *req.MatchRegex
	}
	if req.FirstTokenTimeOut != nil {
		selectFields = append(selectFields, "first_token_time_out")
		updates.FirstTokenTimeOut = *req.FirstTokenTimeOut
	}

	if len(selectFields) > 0 {
		if err := tx.Model(&model.Group{}).Where("id = ?", req.ID).Select(selectFields).Updates(&updates).Error; err != nil {
			tx.Rollback()
			return nil, fmt.Errorf("failed to update group: %w", err)
		}
	}

	// 删除 items
	if len(req.ItemsToDelete) > 0 {
		if err := tx.Where("id IN ? AND group_id = ?", req.ItemsToDelete, req.ID).Delete(&model.GroupItem{}).Error; err != nil {
			tx.Rollback()
			return nil, fmt.Errorf("failed to delete items: %w", err)
		}
	}

	// 批量更新 items
	if len(req.ItemsToUpdate) > 0 {
		ids := make([]int, len(req.ItemsToUpdate))
		priorityCase := "CASE id"
		weightCase := "CASE id"
		for i, item := range req.ItemsToUpdate {
			ids[i] = item.ID
			priorityCase += fmt.Sprintf(" WHEN %d THEN %d", item.ID, item.Priority)
			weightCase += fmt.Sprintf(" WHEN %d THEN %d", item.ID, item.Weight)
		}
		priorityCase += " END"
		weightCase += " END"

		if err := tx.Model(&model.GroupItem{}).
			Where("id IN ? AND group_id = ?", ids, req.ID).
			Updates(map[string]interface{}{
				"priority": gorm.Expr(priorityCase),
				"weight":   gorm.Expr(weightCase),
			}).Error; err != nil {
			tx.Rollback()
			return nil, fmt.Errorf("failed to update items: %w", err)
		}
	}

	// 批量新增 items
	if len(req.ItemsToAdd) > 0 {
		newItems := make([]model.GroupItem, len(req.ItemsToAdd))
		for i, item := range req.ItemsToAdd {
			newItems[i] = model.GroupItem{
				GroupID:   req.ID,
				ChannelID: item.ChannelID,
				ModelName: item.ModelName,
				Priority:  item.Priority,
				Weight:    item.Weight,
			}
		}
		if err := tx.Create(&newItems).Error; err != nil {
			tx.Rollback()
			return nil, fmt.Errorf("failed to create items: %w", err)
		}
	}

	if err := tx.Commit().Error; err != nil {
		return nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// 刷新缓存并返回最新数据
	if err := groupRefreshCacheByID(req.ID, ctx); err != nil {
		return nil, err
	}

	group, _ := groupCache.Get(req.ID)
	if oldName != "" && oldName != group.Name {
		groupMap.Del(oldName)
	}
	return &group, nil
}

func GroupDel(id int, ctx context.Context) error {
	group, ok := groupCache.Get(id)
	if !ok {
		return fmt.Errorf("group not found")
	}

	tx := db.GetDB().WithContext(ctx).Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	if err := tx.Where("group_id = ?", id).Delete(&model.GroupItem{}).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to delete group items: %w", err)
	}

	if err := tx.Delete(&model.Group{}, id).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to delete group: %w", err)
	}

	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	groupCache.Del(id)
	groupMap.Del(group.Name)
	return nil
}

func GroupItemAdd(item *model.GroupItem, ctx context.Context) error {
	if _, ok := groupCache.Get(item.GroupID); !ok {
		return fmt.Errorf("group not found")
	}

	if err := db.GetDB().WithContext(ctx).Create(item).Error; err != nil {
		return err
	}

	return groupRefreshCacheByID(item.GroupID, ctx)
}

func GroupItemBatchAdd(groupID int, items []model.GroupIDAndLLMName, ctx context.Context) error {
	if len(items) == 0 {
		return nil
	}

	group, ok := groupCache.Get(groupID)
	if !ok {
		return fmt.Errorf("group not found")
	}

	seen := make(map[string]struct{}, len(items))
	uniq := make([]model.GroupIDAndLLMName, 0, len(items))
	for _, it := range items {
		if it.ChannelID == 0 || it.ModelName == "" {
			continue
		}
		k := fmt.Sprintf("%d|%s", it.ChannelID, it.ModelName)
		if _, exists := seen[k]; exists {
			continue
		}
		seen[k] = struct{}{}
		uniq = append(uniq, it)
	}
	if len(uniq) == 0 {
		return nil
	}

	nextPriority := 1
	for _, gi := range group.Items {
		if gi.Priority >= nextPriority {
			nextPriority = gi.Priority + 1
		}
	}

	newItems := make([]model.GroupItem, 0, len(uniq))
	for _, it := range uniq {
		newItems = append(newItems, model.GroupItem{
			GroupID:   groupID,
			ChannelID: it.ChannelID,
			ModelName: it.ModelName,
			Priority:  nextPriority,
			Weight:    1,
		})
		nextPriority++
	}

	if err := db.GetDB().WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "group_id"}, {Name: "channel_id"}, {Name: "model_name"}},
			DoNothing: true,
		}).
		Create(&newItems).Error; err != nil {
		return fmt.Errorf("failed to create group items: %w", err)
	}

	return groupRefreshCacheByID(groupID, ctx)
}

func GroupItemUpdate(item *model.GroupItem, ctx context.Context) error {
	if err := db.GetDB().WithContext(ctx).Model(item).
		Select("ModelName", "Priority", "Weight").
		Updates(item).Error; err != nil {
		return err
	}

	return groupRefreshCacheByID(item.GroupID, ctx)
}

func GroupItemDel(id int, ctx context.Context) error {
	var item model.GroupItem
	if err := db.GetDB().WithContext(ctx).First(&item, id).Error; err != nil {
		return fmt.Errorf("group item not found")
	}

	if err := db.GetDB().WithContext(ctx).Delete(&item).Error; err != nil {
		return err
	}

	return groupRefreshCacheByID(item.GroupID, ctx)
}

// GroupItemBatchDelByChannelAndModels 根据渠道ID和模型名称批量删除分组项
func GroupItemBatchDelByChannelAndModels(keys []model.GroupIDAndLLMName, ctx context.Context) error {
	if len(keys) == 0 {
		return nil
	}

	conditions := make([][]interface{}, len(keys))
	for i, key := range keys {
		conditions[i] = []interface{}{key.ChannelID, key.ModelName}
	}

	var groupIDs []int
	if err := db.GetDB().WithContext(ctx).
		Model(&model.GroupItem{}).
		Distinct("group_id").
		Where("(channel_id, model_name) IN ?", conditions).
		Pluck("group_id", &groupIDs).Error; err != nil {
		return fmt.Errorf("failed to find group ids: %w", err)
	}

	if len(groupIDs) == 0 {
		return nil
	}

	if err := db.GetDB().WithContext(ctx).
		Where("(channel_id, model_name) IN ?", conditions).
		Delete(&model.GroupItem{}).Error; err != nil {
		return fmt.Errorf("failed to delete group items: %w", err)
	}

	if err := groupRefreshCacheByIDs(groupIDs, ctx); err != nil {
		return fmt.Errorf("failed to refresh group cache: %w", err)
	}

	return nil
}

func GroupItemList(groupID int, ctx context.Context) ([]model.GroupItem, error) {
	var items []model.GroupItem
	if err := db.GetDB().WithContext(ctx).
		Where("group_id = ?", groupID).
		Order("priority ASC").
		Find(&items).Error; err != nil {
		return nil, err
	}
	return items, nil
}

func groupRefreshCache(ctx context.Context) error {
	groups := []model.Group{}
	if err := db.GetDB().WithContext(ctx).
		Preload("Items").
		Find(&groups).Error; err != nil {
		return err
	}
	for _, group := range groups {
		groupCache.Set(group.ID, group)
		groupMap.Set(group.Name, group)
	}
	return nil
}

func groupRefreshCacheByID(id int, ctx context.Context) error {
	var group model.Group
	if err := db.GetDB().WithContext(ctx).
		Preload("Items").
		First(&group, id).Error; err != nil {
		return err
	}
	groupCache.Set(group.ID, group)
	groupMap.Set(group.Name, group)
	return nil
}

func groupRefreshCacheByIDs(ids []int, ctx context.Context) error {
	if len(ids) == 0 {
		return nil
	}
	var groups []model.Group
	if err := db.GetDB().WithContext(ctx).
		Preload("Items").
		Where("id IN ?", ids).
		Find(&groups).Error; err != nil {
		return err
	}
	for _, group := range groups {
		groupCache.Set(group.ID, group)
		groupMap.Set(group.Name, group)
	}
	return nil
}

// GroupItemPromote promotes the priority of a group item (smaller priority value means higher priority)
// via swapping with the item that has the immediately higher priority (structurally lower value).
func GroupItemPromote(item *model.GroupItem, ctx context.Context) error {
	group, ok := groupCache.Get(item.GroupID)
	if !ok {
		// Try to load from DB if not in cache
		if err := groupRefreshCacheByID(item.GroupID, ctx); err != nil {
			return err
		}
		gPtr, errVal := GroupGet(item.GroupID, ctx)
		if errVal != nil {
			return fmt.Errorf("group not found: %w", errVal)
		}
		group = *gPtr
	}

	// We need to work with a sorted list of items to find the predecessor
	items := make([]model.GroupItem, len(group.Items))
	copy(items, group.Items)

	// Sort by priority (ascending)
	sort.Slice(items, func(i, j int) bool {
		return items[i].Priority < items[j].Priority
	})

	// Find the index of our item
	var currentIndex = -1
	for i, it := range items {
		if it.ID == item.ID {
			currentIndex = i
			break
		}
	}

	if currentIndex == -1 {
		return fmt.Errorf("item not found in group")
	}

	// If it's already at index 0, it has the highest priority (lowest value), so nothing to do.
	if currentIndex == 0 {
		return nil
	}

	// Identify the target to swap with (the one immediately 'before' it in the sorted list)
	targetUser := items[currentIndex-1]
	currentUser := items[currentIndex]

	// Use Failover (Priority) logic for selection, but here we want to SWAP priorities to boost 'currentUser'
	// and lower 'targetUser'.

	newCurrentPriority := targetUser.Priority
	newTargetPriority := currentUser.Priority

	// If priorities are equal, simply swapping values (which are identical) won't change the sort order.
	// To ensure 'currentUser' is promoted above 'targetUser', we need to make its priority strictly better (smaller).
	if newCurrentPriority == newTargetPriority {
		newCurrentPriority--
	}

	tx := db.GetDB().WithContext(ctx).Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Update currentUser to have target's priority
	if err := tx.Model(&model.GroupItem{}).Where("id = ?", currentUser.ID).Update("priority", newCurrentPriority).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to update current item priority: %w", err)
	}

	// Update targetUser to have currentUser's old priority
	if err := tx.Model(&model.GroupItem{}).Where("id = ?", targetUser.ID).Update("priority", newTargetPriority).Error; err != nil {
		tx.Rollback()
		return fmt.Errorf("failed to update target item priority: %w", err)
	}

	if err := tx.Commit().Error; err != nil {
		return fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Refresh cache
	return groupRefreshCacheByID(item.GroupID, ctx)
}
