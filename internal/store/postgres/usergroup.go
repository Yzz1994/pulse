package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"pulse/internal/store/postgres/sqlcgen"
	"pulse/internal/usergroup"
)

// UserGroupStore 实现 usergroup.Store 接口。
type UserGroupStore struct {
	db *pgxpool.Pool
}

func (s *UserGroupStore) UpsertUserGroup(g usergroup.UserGroup) (usergroup.UserGroup, error) {
	err := sqlcgen.New(s.db).UpsertUserGroup(context.Background(), sqlcgen.UpsertUserGroupParams{
		ID:         g.ID,
		Name:       g.Name,
		Remark:     g.Remark,
		InboundIds: g.InboundIDs,
	})
	if err != nil {
		return usergroup.UserGroup{}, fmt.Errorf("upsert user group: %w", err)
	}
	return g, nil
}

func (s *UserGroupStore) GetUserGroup(id string) (usergroup.UserGroup, error) {
	row, err := sqlcgen.New(s.db).GetUserGroupByID(context.Background(), id)
	if errors.Is(err, pgx.ErrNoRows) {
		return usergroup.UserGroup{}, usergroup.ErrUserGroupNotFound
	}
	if err != nil {
		return usergroup.UserGroup{}, fmt.Errorf("get user group: %w", err)
	}
	return usergroup.UserGroup{
		ID:         row.ID,
		Name:       row.Name,
		Remark:     row.Remark,
		InboundIDs: row.InboundIds,
	}, nil
}

func (s *UserGroupStore) ListUserGroups() ([]usergroup.UserGroup, error) {
	rows, err := sqlcgen.New(s.db).ListUserGroups(context.Background())
	if err != nil {
		return nil, fmt.Errorf("list user groups: %w", err)
	}
	out := make([]usergroup.UserGroup, len(rows))
	for i, r := range rows {
		out[i] = usergroup.UserGroup{
			ID:         r.ID,
			Name:       r.Name,
			Remark:     r.Remark,
			InboundIDs: r.InboundIds,
		}
	}
	return out, nil
}

func (s *UserGroupStore) DeleteUserGroup(id string) error {
	res, err := sqlcgen.New(s.db).DeleteUserGroupByID(context.Background(), id)
	if err != nil {
		return fmt.Errorf("delete user group: %w", err)
	}
	if res.RowsAffected() == 0 {
		return usergroup.ErrUserGroupNotFound
	}
	return nil
}

func (s *UserGroupStore) AddMember(groupID, userID string) error {
	err := sqlcgen.New(s.db).AddUserGroupMember(context.Background(), sqlcgen.AddUserGroupMemberParams{
		GroupID: groupID,
		UserID:  userID,
	})
	if err != nil {
		return fmt.Errorf("add user group member: %w", err)
	}
	return nil
}

func (s *UserGroupStore) RemoveMember(groupID, userID string) error {
	err := sqlcgen.New(s.db).RemoveUserGroupMember(context.Background(), sqlcgen.RemoveUserGroupMemberParams{
		GroupID: groupID,
		UserID:  userID,
	})
	if err != nil {
		return fmt.Errorf("remove user group member: %w", err)
	}
	return nil
}

func (s *UserGroupStore) ListGroupMembers(groupID string) ([]string, error) {
	rows, err := sqlcgen.New(s.db).ListUserGroupMembers(context.Background(), groupID)
	if err != nil {
		return nil, fmt.Errorf("list user group members: %w", err)
	}
	return rows, nil
}

func (s *UserGroupStore) ListUserGroupIDs(userID string) ([]string, error) {
	rows, err := sqlcgen.New(s.db).ListUserGroupsByUser(context.Background(), userID)
	if err != nil {
		return nil, fmt.Errorf("list user groups by user: %w", err)
	}
	return rows, nil
}
