// Package rank 提供排行榜管理功能
// - 支持按得分从高到低排序
// - 支持按用户工号刷新排行数据
// - 数据持久化到本地文件
// - 服务启动时加载所有排行数据到内存
package rank

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

const (
	rankDataFile  = "rank.json"
	tempSuffix    = ".tmp"
	backupSuffix  = ".backup"
)

// RankItem 排行榜单项数据
type RankItem struct {
	Rank          int       `json:"rank"`           // 排名（动态计算，不持久化）
	TeamName      string    `json:"team_name"`      // 队伍名
	UserID        string    `json:"user_id"`        // 工号
	Username      string    `json:"username"`       // 姓名
	Score         int       `json:"score"`          // 得分
	CompletedTasks int      `json:"completed_tasks"` // 完成任务数
	UpdateTime    time.Time `json:"update_time"`    // 更新时间
}

// RankUpdateRequest 排行更新请求（内部使用）
type RankUpdateRequest struct {
	TeamName       string `json:"team_name"`
	Username       string `json:"username"`
	Score          int    `json:"score"`
	CompletedTasks int    `json:"completed_tasks"`
	AddScore       int    `json:"add_score"`        // 增量得分（可选）
	AddTasks       int    `json:"add_tasks"`        // 增量任务数（可选）
}

// RankManager 排行榜管理器
type RankManager struct {
	dataDir   string
	items     map[string]*RankItem  // 以userID为key的排行数据
	sorted    []*RankItem           // 排序后的排行列表
	mu        sync.RWMutex
	dataFile  string
	lastSort  time.Time
}

// NewRankManager 创建新的排行榜管理器
func NewRankManager(dataDir string) (*RankManager, error) {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("创建排行榜数据目录失败: %w", err)
	}

	rm := &RankManager{
		dataDir:  dataDir,
		items:    make(map[string]*RankItem),
		sorted:   make([]*RankItem, 0),
		dataFile: filepath.Join(dataDir, rankDataFile),
	}

	// 从磁盘加载排行数据
	if err := rm.loadData(); err != nil {
		return nil, fmt.Errorf("加载排行榜数据失败: %w", err)
	}

	log.Printf("[RankManager] 初始化完成，已加载 %d 条排行数据", len(rm.items))
	return rm, nil
}

// rankDataFileStruct 持久化数据结构
type rankDataFileStruct struct {
	Version  int          `json:"version"`
	UpdateAt int64        `json:"update_at"`
	Items    []*RankItem  `json:"items"`
}

// loadData 从磁盘加载排行数据
func (rm *RankManager) loadData() error {
	if _, err := os.Stat(rm.dataFile); os.IsNotExist(err) {
		log.Printf("[RankManager] 排行数据文件不存在，创建新的")
		return rm.saveData()
	}

	data, err := os.ReadFile(rm.dataFile)
	if err != nil {
		return fmt.Errorf("读取排行数据文件失败: %w", err)
	}

	if !json.Valid(data) {
		return fmt.Errorf("排行数据文件JSON格式无效")
	}

	var fileData rankDataFileStruct
	if err := json.Unmarshal(data, &fileData); err != nil {
		return fmt.Errorf("解析排行数据失败: %w", err)
	}

	// 加载到内存
	for _, item := range fileData.Items {
		if item.UserID == "" {
			continue
		}
		rm.items[item.UserID] = item
	}

	// 重新计算排名
	rm.sortItems()

	return nil
}

// saveData 保存排行数据到磁盘（原子写入）
func (rm *RankManager) saveData() error {
	fileData := rankDataFileStruct{
		Version:  1,
		UpdateAt: time.Now().Unix(),
		Items:    make([]*RankItem, 0, len(rm.items)),
	}

	for _, item := range rm.items {
		fileData.Items = append(fileData.Items, item)
	}

	data, err := json.MarshalIndent(fileData, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化排行数据失败: %w", err)
	}

	// 原子写入
	tempFile := rm.dataFile + tempSuffix + "." + fmt.Sprintf("%d", time.Now().UnixNano())

	f, err := os.OpenFile(tempFile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("创建临时文件失败: %w", err)
	}

	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(tempFile)
		return fmt.Errorf("写入临时文件失败: %w", err)
	}

	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tempFile)
		return fmt.Errorf("同步文件到磁盘失败: %w", err)
	}

	if err := f.Close(); err != nil {
		os.Remove(tempFile)
		return fmt.Errorf("关闭临时文件失败: %w", err)
	}

	if err := os.Rename(tempFile, rm.dataFile); err != nil {
		os.Remove(tempFile)
		return fmt.Errorf("重命名文件失败: %w", err)
	}

	// 同步目录
	if dirFile, err := os.Open(rm.dataDir); err == nil {
		dirFile.Sync()
		dirFile.Close()
	}

	return nil
}

// sortItems 对排行数据进行排序（按得分从高到低）
func (rm *RankManager) sortItems() {
	rm.sorted = make([]*RankItem, 0, len(rm.items))
	for _, item := range rm.items {
		itemCopy := *item
		rm.sorted = append(rm.sorted, &itemCopy)
	}

	// 按得分降序排序，得分相同按完成任务数降序，再相同按更新时间升序
	sort.Slice(rm.sorted, func(i, j int) bool {
		if rm.sorted[i].Score != rm.sorted[j].Score {
			return rm.sorted[i].Score > rm.sorted[j].Score
		}
		if rm.sorted[i].CompletedTasks != rm.sorted[j].CompletedTasks {
			return rm.sorted[i].CompletedTasks > rm.sorted[j].CompletedTasks
		}
		return rm.sorted[i].UpdateTime.Before(rm.sorted[j].UpdateTime)
	})

	// 重新计算排名
	for i, item := range rm.sorted {
		item.Rank = i + 1
	}

	rm.lastSort = time.Now()
}

// ensureSorted 确保数据已排序
func (rm *RankManager) ensureSorted() {
	// 如果数据有变化，需要重新排序
	// 简单实现：每次检查排序时间，超过1秒则重新排序
	if time.Since(rm.lastSort) > time.Second {
		rm.sortItems()
	}
}

// UpdateOrCreate 更新或创建排行榜项（内部接口）
func (rm *RankManager) UpdateOrCreate(userID string, req *RankUpdateRequest) error {
	if userID == "" {
		return fmt.Errorf("用户工号不能为空")
	}

	rm.mu.Lock()
	defer rm.mu.Unlock()

	item, exists := rm.items[userID]
	if !exists {
		// 创建新项
		item = &RankItem{
			UserID:   userID,
			UpdateTime: time.Now(),
		}
	}

	// 更新字段
	if req.TeamName != "" {
		item.TeamName = req.TeamName
	}
	if req.Username != "" {
		item.Username = req.Username
	}
	if req.Score > 0 {
		item.Score = req.Score
	}
	if req.CompletedTasks > 0 {
		item.CompletedTasks = req.CompletedTasks
	}
	// 增量更新
	if req.AddScore != 0 {
		item.Score += req.AddScore
		if item.Score < 0 {
			item.Score = 0
		}
	}
	if req.AddTasks != 0 {
		item.CompletedTasks += req.AddTasks
		if item.CompletedTasks < 0 {
			item.CompletedTasks = 0
		}
	}

	item.UpdateTime = time.Now()
	rm.items[userID] = item

	// 持久化到磁盘
	if err := rm.saveData(); err != nil {
		return fmt.Errorf("保存排行数据失败: %w", err)
	}

	// 标记需要重新排序
	rm.lastSort = time.Time{}

	log.Printf("[RankManager] 用户 %s 排行数据已更新: score=%d, tasks=%d",
		userID, item.Score, item.CompletedTasks)
	return nil
}

// RefreshUser 按用户工号刷新用户排行数据（内部接口）
func (rm *RankManager) RefreshUser(userID string) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if _, exists := rm.items[userID]; !exists {
		return fmt.Errorf("用户不存在于排行榜: %s", userID)
	}

	// 更新时间戳，触发重新排序
	rm.items[userID].UpdateTime = time.Now()

	// 持久化
	if err := rm.saveData(); err != nil {
		return fmt.Errorf("保存排行数据失败: %w", err)
	}

	rm.lastSort = time.Time{}
	log.Printf("[RankManager] 用户 %s 排行数据已刷新", userID)
	return nil
}

// GetRankList 获取排行榜列表（对外接口，按得分从高到低排序）
func (rm *RankManager) GetRankList(limit int) []*RankItem {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	rm.ensureSorted()

	if limit <= 0 || limit > len(rm.sorted) {
		limit = len(rm.sorted)
	}

	// 返回副本
	result := make([]*RankItem, limit)
	for i := 0; i < limit; i++ {
		itemCopy := *rm.sorted[i]
		result[i] = &itemCopy
	}

	return result
}

// GetUserRank 获取单个用户排行信息
func (rm *RankManager) GetUserRank(userID string) (*RankItem, error) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	rm.ensureSorted()

	if _, exists := rm.items[userID]; !exists {
		return nil, fmt.Errorf("用户不在排行榜中: %s", userID)
	}

	// 查找当前排名
	for _, sortedItem := range rm.sorted {
		if sortedItem.UserID == userID {
			itemCopy := *sortedItem
			return &itemCopy, nil
		}
	}

	return nil, fmt.Errorf("无法获取用户排名: %s", userID)
}

// GetRankCount 获取排行榜总人数
func (rm *RankManager) GetRankCount() int {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	return len(rm.items)
}

// DeleteUser 从排行榜删除用户（内部接口）
func (rm *RankManager) DeleteUser(userID string) error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	if _, exists := rm.items[userID]; !exists {
		return fmt.Errorf("用户不存在于排行榜: %s", userID)
	}

	delete(rm.items, userID)

	// 持久化
	if err := rm.saveData(); err != nil {
		return fmt.Errorf("保存排行数据失败: %w", err)
	}

	rm.lastSort = time.Time{}
	log.Printf("[RankManager] 用户 %s 已从排行榜删除", userID)
	return nil
}

// ReloadData 重新从磁盘加载排行数据
func (rm *RankManager) ReloadData() error {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	// 清空内存
	rm.items = make(map[string]*RankItem)
	rm.sorted = make([]*RankItem, 0)

	// 重新加载
	if err := rm.loadData(); err != nil {
		return fmt.Errorf("重新加载排行数据失败: %w", err)
	}

	log.Printf("[RankManager] 重新加载完成，当前 %d 条排行数据", len(rm.items))
	return nil
}
