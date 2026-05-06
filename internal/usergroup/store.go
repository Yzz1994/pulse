package usergroup

import "errors"

// ErrUserGroupNotFound 用户组不存在时返回。
var ErrUserGroupNotFound = errors.New("user group not found")

// UserGroup 用户组定义。
type UserGroup struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Remark     string `json:"remark"`
	InboundIDs string `json:"inbound_ids"` // 逗号分隔的 inbound ID 列表
}

// Store 用户组的持久化接口。
type Store interface {
	UpsertUserGroup(g UserGroup) (UserGroup, error)
	GetUserGroup(id string) (UserGroup, error)
	ListUserGroups() ([]UserGroup, error)
	DeleteUserGroup(id string) error
	AddMember(groupID, userID string) error
	RemoveMember(groupID, userID string) error
	ListGroupMembers(groupID string) ([]string, error) // 返回 userIDs
	ListUserGroupIDs(userID string) ([]string, error)  // 返回 groupIDs
}
