// Package user 提供用户信息管理功能，具备高可靠性机制
// - 原子文件写入：使用临时文件+重命名，防止写入中断导致数据损坏
// - 数据一致性校验：验证用户ID、必填字段、数据格式
// - 备份机制：更新用户前自动备份原文件
// - 强制落盘：使用 Sync() 确保数据写入磁盘
// - 服务启动全量加载：启动时将所有用户信息加载到内存
package user

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
	// 备份文件后缀
	backupSuffix = ".backup"
	// 临时文件后缀
	tempSuffix = ".tmp"
	// 用户清单文件名
	usersListFile = "users.json"
	// 用户数据文件名
	userDataFile = "user.json"
)

// User 用户结构体
type User struct {
	UserID    string `json:"user_id"`    // 用户工号（唯一标识）
	Username  string `json:"username"`   // 用户名
	TeamName  string `json:"team_name"`  // 队伍名
	AgentIP   string `json:"agent_ip"`   // Agent IP 地址
	AgentPort int    `json:"agent_port"` // Agent 端口号
	// 内部字段，不导出到JSON
	createTime time.Time `json:"-"`
	updateTime time.Time `json:"-"`
}

// UserManager 用户管理器
type UserManager struct {
	workspaceDir string           // 工作空间根目录
	users        map[string]*User // 内存中的用户缓存
	mu           sync.RWMutex     // 读写锁
	usersFile    string           // 用户清单文件路径
}

// usersList 用户清单结构
type usersList struct {
	Users    []string          `json:"users"`
	Version  int               `json:"version"`  // 数据版本号，用于数据迁移
	UpdateAt int64             `json:"update_at"` // 最后更新时间戳
}

// NewUserManager 创建新的用户管理器，启动时将所有用户信息加载到内存
func NewUserManager(workspaceDir string) (*UserManager, error) {
	// 创建工作空间目录（如果不存在）
	if err := os.MkdirAll(workspaceDir, 0755); err != nil {
		return nil, fmt.Errorf("创建工作空间目录失败: %w", err)
	}

	um := &UserManager{
		workspaceDir: workspaceDir,
		users:        make(map[string]*User),
		usersFile:    filepath.Join(workspaceDir, usersListFile),
	}

	// 从磁盘加载用户信息到内存
	if err := um.loadUsers(); err != nil {
		return nil, fmt.Errorf("加载用户信息失败: %w", err)
	}

	log.Printf("[UserManager] 初始化完成，已加载 %d 个用户", len(um.users))
	return um, nil
}

// loadUsers 从磁盘加载所有用户信息到内存
func (um *UserManager) loadUsers() error {
	// 检查用户清单文件是否存在
	if _, err := os.Stat(um.usersFile); os.IsNotExist(err) {
		// 文件不存在，创建空的用户清单
		log.Printf("[UserManager] 用户清单不存在，创建新的清单")
		return um.saveUsersList()
	}

	// 读取用户清单
	data, err := os.ReadFile(um.usersFile)
	if err != nil {
		return fmt.Errorf("读取用户清单失败: %w", err)
	}

	var list usersList
	if err := json.Unmarshal(data, &list); err != nil {
		return fmt.Errorf("解析用户清单失败: %w", err)
	}

	// 加载每个用户的信息
	loadedCount := 0
	errorCount := 0
	for _, userID := range list.Users {
		user, err := um.loadUserFromFile(userID)
		if err != nil {
			log.Printf("[UserManager] 警告: 加载用户 %s 失败: %v", userID, err)
			errorCount++
			continue
		}

		// 数据一致性校验
		if err := um.validateUser(user); err != nil {
			log.Printf("[UserManager] 警告: 用户 %s 数据校验失败: %v", userID, err)
			errorCount++
			continue
		}

		// 确保用户ID与文件路径一致
		if user.UserID != userID {
			log.Printf("[UserManager] 警告: 用户文件 %s 中的ID(%s)与文件名不匹配", userID, user.UserID)
			user.UserID = userID // 修正用户ID
		}

		um.users[userID] = user
		loadedCount++
	}

	if errorCount > 0 {
		log.Printf("[UserManager] 加载完成: 成功 %d 个, 失败 %d 个", loadedCount, errorCount)
	}

	return nil
}

// loadUserFromFile 从文件加载单个用户信息
func (um *UserManager) loadUserFromFile(userID string) (*User, error) {
	userFile := filepath.Join(um.workspaceDir, userID, userDataFile)

	// 检查文件是否存在
	if _, err := os.Stat(userFile); os.IsNotExist(err) {
		return nil, fmt.Errorf("用户文件不存在: %s", userFile)
	}

	data, err := os.ReadFile(userFile)
	if err != nil {
		return nil, fmt.Errorf("读取用户文件失败: %w", err)
	}

	// 验证JSON格式
	if !json.Valid(data) {
		return nil, fmt.Errorf("用户文件JSON格式无效")
	}

	var user User
	if err := json.Unmarshal(data, &user); err != nil {
		return nil, fmt.Errorf("解析用户文件失败: %w", err)
	}

	// 记录加载时间
	user.createTime = time.Now()
	user.updateTime = time.Now()

	return &user, nil
}

// validateUser 验证用户数据完整性
func (um *UserManager) validateUser(user *User) error {
	if user.UserID == "" {
		return fmt.Errorf("用户工号不能为空")
	}
	if user.Username == "" {
		return fmt.Errorf("用户名不能为空")
	}
	if user.AgentIP == "" {
		return fmt.Errorf("Agent IP 不能为空")
	}
	if user.AgentPort <= 0 || user.AgentPort > 65535 {
		return fmt.Errorf("Agent 端口无效: %d", user.AgentPort)
	}
	return nil
}

// atomicWriteFile 原子写入文件（先写临时文件，再重命名）
func (um *UserManager) atomicWriteFile(filePath string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(filePath)

	// 创建临时文件
	tempFile := filePath + tempSuffix + "." + fmt.Sprintf("%d", time.Now().UnixNano())

	f, err := os.OpenFile(tempFile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return fmt.Errorf("创建临时文件失败: %w", err)
	}

	// 写入数据
	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(tempFile)
		return fmt.Errorf("写入临时文件失败: %w", err)
	}

	// 强制同步到磁盘
	if err := f.Sync(); err != nil {
		f.Close()
		os.Remove(tempFile)
		return fmt.Errorf("同步文件到磁盘失败: %w", err)
	}

	// 关闭文件
	if err := f.Close(); err != nil {
		os.Remove(tempFile)
		return fmt.Errorf("关闭临时文件失败: %w", err)
	}

	// 原子重命名
	if err := os.Rename(tempFile, filePath); err != nil {
		os.Remove(tempFile)
		return fmt.Errorf("重命名文件失败: %w", err)
	}

	// 同步目录，确保重命名操作持久化
	dirFile, err := os.Open(dir)
	if err == nil {
		dirFile.Sync()
		dirFile.Close()
	}

	return nil
}

// backupFile 备份文件
func (um *UserManager) backupFile(filePath string) error {
	backupPath := filePath + backupSuffix
	data, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // 原文件不存在，无需备份
		}
		return fmt.Errorf("读取原文件失败: %w", err)
	}

	if err := os.WriteFile(backupPath, data, 0644); err != nil {
		return fmt.Errorf("写入备份文件失败: %w", err)
	}

	return nil
}

// saveUserToFile 保存单个用户信息到文件（原子写入）
func (um *UserManager) saveUserToFile(user *User) error {
	// 数据校验
	if err := um.validateUser(user); err != nil {
		return fmt.Errorf("用户数据校验失败: %w", err)
	}

	userDir := filepath.Join(um.workspaceDir, user.UserID)
	if err := os.MkdirAll(userDir, 0755); err != nil {
		return fmt.Errorf("创建用户目录失败: %w", err)
	}

	userFile := filepath.Join(userDir, userDataFile)

	// 更新修改时间
	user.updateTime = time.Now()

	data, err := json.MarshalIndent(user, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化用户信息失败: %w", err)
	}

	// 原子写入
	if err := um.atomicWriteFile(userFile, data, 0644); err != nil {
		return fmt.Errorf("写入用户文件失败: %w", err)
	}

	log.Printf("[UserManager] 用户 %s 数据已持久化到磁盘", user.UserID)
	return nil
}

// saveUsersList 保存用户清单到文件（原子写入）
func (um *UserManager) saveUsersList() error {
	// 构建用户列表（排序保证一致性）
	userIDs := make([]string, 0, len(um.users))
	for userID := range um.users {
		userIDs = append(userIDs, userID)
	}
	sort.Strings(userIDs)

	list := usersList{
		Users:    userIDs,
		Version:  1,
		UpdateAt: time.Now().Unix(),
	}

	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化用户清单失败: %w", err)
	}

	// 原子写入
	if err := um.atomicWriteFile(um.usersFile, data, 0644); err != nil {
		return fmt.Errorf("写入用户清单失败: %w", err)
	}

	return nil
}

// AddUser 添加用户，即时刷新本地文件
func (um *UserManager) AddUser(user *User) error {
	um.mu.Lock()
	defer um.mu.Unlock()

	// 数据校验
	if err := um.validateUser(user); err != nil {
		return err
	}

	// 检查用户是否已存在
	if _, exists := um.users[user.UserID]; exists {
		return fmt.Errorf("用户工号已存在: %s", user.UserID)
	}

	// 第一步：保存用户文件到磁盘（原子写入）
	if err := um.saveUserToFile(user); err != nil {
		return fmt.Errorf("保存用户文件失败: %w", err)
	}

	// 第二步：更新内存缓存
	um.users[user.UserID] = user

	// 第三步：更新用户清单（原子写入）
	if err := um.saveUsersList(); err != nil {
		// 回滚操作：从内存中移除，删除用户文件
		delete(um.users, user.UserID)
		os.RemoveAll(filepath.Join(um.workspaceDir, user.UserID))
		return fmt.Errorf("更新用户清单失败，已回滚: %w", err)
	}

	log.Printf("[UserManager] 用户 %s 添加成功", user.UserID)
	return nil
}

// GetUser 从内存获取用户信息
func (um *UserManager) GetUser(userID string) (*User, error) {
	um.mu.RLock()
	defer um.mu.RUnlock()

	user, exists := um.users[userID]
	if !exists {
		return nil, fmt.Errorf("用户不存在: %s", userID)
	}

	// 返回副本，防止外部修改
	userCopy := *user
	return &userCopy, nil
}

// GetAllUsers 从内存获取所有用户（已排序）
func (um *UserManager) GetAllUsers() []*User {
	um.mu.RLock()
	defer um.mu.RUnlock()

	// 按UserID排序
	userIDs := make([]string, 0, len(um.users))
	for userID := range um.users {
		userIDs = append(userIDs, userID)
	}
	sort.Strings(userIDs)

	result := make([]*User, 0, len(um.users))
	for _, userID := range userIDs {
		userCopy := *um.users[userID]
		result = append(result, &userCopy)
	}

	return result
}

// UpdateUser 更新用户信息，即时刷新本地文件
func (um *UserManager) UpdateUser(userID string, updates *User) error {
	um.mu.Lock()
	defer um.mu.Unlock()

	user, exists := um.users[userID]
	if !exists {
		return fmt.Errorf("用户不存在: %s", userID)
	}

	// 备份原文件
	userFile := filepath.Join(um.workspaceDir, userID, userDataFile)
	if err := um.backupFile(userFile); err != nil {
		log.Printf("[UserManager] 警告: 备份用户 %s 原文件失败: %v", userID, err)
	}

	// 创建更新后的用户对象（保留原用户ID）
	updatedUser := &User{
		UserID:     userID,
		Username:   user.Username,
		TeamName:   user.TeamName,
		AgentIP:    user.AgentIP,
		AgentPort:  user.AgentPort,
		createTime: user.createTime,
	}

	// 应用更新（允许部分更新）
	if updates.Username != "" {
		updatedUser.Username = updates.Username
	}
	if updates.TeamName != "" {
		updatedUser.TeamName = updates.TeamName
	}
	if updates.AgentIP != "" {
		updatedUser.AgentIP = updates.AgentIP
	}
	if updates.AgentPort > 0 && updates.AgentPort <= 65535 {
		updatedUser.AgentPort = updates.AgentPort
	}

	// 验证更新后的数据
	if err := um.validateUser(updatedUser); err != nil {
		return fmt.Errorf("更新后数据校验失败: %w", err)
	}

	// 保存到文件（原子写入）
	if err := um.saveUserToFile(updatedUser); err != nil {
		return fmt.Errorf("保存更新失败: %w", err)
	}

	// 更新内存缓存
	um.users[userID] = updatedUser

	log.Printf("[UserManager] 用户 %s 更新成功", userID)
	return nil
}

// DeleteUser 删除用户，即时刷新本地文件
func (um *UserManager) DeleteUser(userID string) error {
	um.mu.Lock()
	defer um.mu.Unlock()

	if _, exists := um.users[userID]; !exists {
		return fmt.Errorf("用户不存在: %s", userID)
	}

	// 第一步：从内存中删除
	delete(um.users, userID)

	// 第二步：更新用户清单（原子写入）
	if err := um.saveUsersList(); err != nil {
		// 恢复内存状态（从文件重新加载）
		if user, err := um.loadUserFromFile(userID); err == nil {
			um.users[userID] = user
		}
		return fmt.Errorf("更新用户清单失败，已回滚: %w", err)
	}

	// 第三步：删除用户目录
	userDir := filepath.Join(um.workspaceDir, userID)
	if err := os.RemoveAll(userDir); err != nil {
		log.Printf("[UserManager] 警告: 删除用户 %s 目录失败: %v", userID, err)
		// 不返回错误，因为清单已更新
	}

	log.Printf("[UserManager] 用户 %s 删除成功", userID)
	return nil
}

// GetUserWorkspace 获取用户工作空间路径
func (um *UserManager) GetUserWorkspace(userID string) (string, error) {
	um.mu.RLock()
	defer um.mu.RUnlock()

	if _, exists := um.users[userID]; !exists {
		return "", fmt.Errorf("用户不存在: %s", userID)
	}

	return filepath.Join(um.workspaceDir, userID), nil
}

// GetAgentURL 获取用户的 Agent 访问地址
func (um *UserManager) GetAgentURL(userID string) (string, error) {
	user, err := um.GetUser(userID)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("http://%s:%d", user.AgentIP, user.AgentPort), nil
}

// UserExists 检查用户是否存在
func (um *UserManager) UserExists(userID string) bool {
	um.mu.RLock()
	defer um.mu.RUnlock()

	_, exists := um.users[userID]
	return exists
}

// GetUserCount 获取用户数量
func (um *UserManager) GetUserCount() int {
	um.mu.RLock()
	defer um.mu.RUnlock()

	return len(um.users)
}

// ReloadUsers 重新从磁盘加载所有用户（用于数据恢复）
func (um *UserManager) ReloadUsers() error {
	um.mu.Lock()
	defer um.mu.Unlock()

	// 清空内存
	um.users = make(map[string]*User)

	// 重新加载
	if err := um.loadUsers(); err != nil {
		return fmt.Errorf("重新加载用户失败: %w", err)
	}

	log.Printf("[UserManager] 重新加载完成，当前 %d 个用户", len(um.users))
	return nil
}

// RestoreFromBackup 从备份恢复用户数据
func (um *UserManager) RestoreFromBackup(userID string) error {
	um.mu.Lock()
	defer um.mu.Unlock()

	userFile := filepath.Join(um.workspaceDir, userID, userDataFile)
	backupFile := userFile + backupSuffix

	// 检查备份文件是否存在
	if _, err := os.Stat(backupFile); os.IsNotExist(err) {
		return fmt.Errorf("备份文件不存在: %s", backupFile)
	}

	// 读取备份数据
	data, err := os.ReadFile(backupFile)
	if err != nil {
		return fmt.Errorf("读取备份文件失败: %w", err)
	}

	// 验证备份数据
	var user User
	if err := json.Unmarshal(data, &user); err != nil {
		return fmt.Errorf("备份数据格式无效: %w", err)
	}

	// 恢复数据
	if err := um.atomicWriteFile(userFile, data, 0644); err != nil {
		return fmt.Errorf("恢复用户文件失败: %w", err)
	}

	// 更新内存
	um.users[userID] = &user

	log.Printf("[UserManager] 用户 %s 从备份恢复成功", userID)
	return nil
}
