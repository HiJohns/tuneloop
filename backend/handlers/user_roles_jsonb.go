package handlers

import (
	"encoding/json"
	"fmt"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// userRolesExpr 把 users.roles 镜像序列化为 jsonb 表达式（#2647 R7）。
//
// 与 siteMemberRolesExpr（site_members.roles）同语义、不同表：GORM 的 map Update
// 绕过 `serializer:json`，直接把 Go `[]string` 交给驱动会被渲染成记录/数组字面量，
// 对 jsonb 列报 SQLSTATE 42804（simple protocol 下必现）。因此所有以 map 形态写
// users.roles 的路径必须复用本 helper，不得各自拼 SQL、也不得把第二份实现拷到别处。
//
// 语义：去空白 + 去重保序；nil/空集合 → `[]`（roles 列 NOT NULL DEFAULT '[]'，
// 禁止写 null —— 读侧按数组消费）。
func userRolesExpr(roles []string) (clause.Expr, error) {
	normalized := make([]string, 0, len(roles))
	seen := make(map[string]struct{}, len(roles))
	for _, role := range roles {
		role = strings.TrimSpace(role)
		if role == "" {
			continue
		}
		if _, ok := seen[role]; ok {
			continue
		}
		seen[role] = struct{}{}
		normalized = append(normalized, role)
	}
	encoded, err := json.Marshal(normalized)
	if err != nil {
		return clause.Expr{}, fmt.Errorf("encode user roles: %w", err)
	}
	return gorm.Expr("?::jsonb", string(encoded)), nil
}

// setUserRolesMirror 把镜像值写入 updates map（失败时返回 error，由调用方决定
// 是否让整条写路径失败 —— 禁止静默丢弃镜像写入）。
func setUserRolesMirror(updates map[string]interface{}, roles []string) error {
	expr, err := userRolesExpr(roles)
	if err != nil {
		return err
	}
	updates["roles"] = expr
	return nil
}

// appendUniqueRoles 返回既有镜像 ∪ 新角色（去空白、去重、保序：既有在前）。
// 改角色是 IAM 侧的 ADD-only 语义（没有 unassign API），因此本地镜像必须并集，
// 不能只用新角色覆盖（#2647 R8/D3）。
func appendUniqueRoles(existing []string, extra ...string) []string {
	merged := make([]string, 0, len(existing)+len(extra))
	seen := make(map[string]struct{}, len(existing)+len(extra))
	for _, role := range existing {
		role = strings.TrimSpace(role)
		if role == "" {
			continue
		}
		if _, ok := seen[role]; ok {
			continue
		}
		seen[role] = struct{}{}
		merged = append(merged, role)
	}
	for _, role := range extra {
		role = strings.TrimSpace(role)
		if role == "" {
			continue
		}
		if _, ok := seen[role]; ok {
			continue
		}
		seen[role] = struct{}{}
		merged = append(merged, role)
	}
	return merged
}
