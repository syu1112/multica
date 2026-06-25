package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

var (
	ErrRuntimeRequesterRequired   = errors.New("runtime resolver: requester required")
	ErrRuntimeNotFound            = errors.New("runtime resolver: runtime not found")
	ErrRuntimeNotOwnedByRequester = errors.New("runtime resolver: runtime not owned by requester")
	ErrRuntimeWorkspaceMismatch   = errors.New("runtime resolver: runtime workspace mismatch")
	ErrRuntimeUnavailable         = errors.New("runtime resolver: runtime unavailable")
	ErrRuntimeCapabilityMismatch  = errors.New("runtime resolver: runtime capability mismatch")
	ErrNoCompatibleRuntimeForUser = errors.New("runtime resolver: no compatible runtime for user")
)

type RuntimeCandidateParams struct {
	WorkspaceID pgtype.UUID
	OwnerID     pgtype.UUID
	Provider    string
	ProfileID   pgtype.UUID
}

type RuntimeResolveInput struct {
	WorkspaceID         pgtype.UUID
	Agent               db.Agent
	RequesterUserID     pgtype.UUID
	// AgentOwnerID 是 agent 的 OwnerID。在 squad 场景中，agent owner 可能
	// 与 requester（issue 创建者/触发者）不同。设置此字段后，resolver 在
	// explicit choice 路径中优先使用 AgentOwnerID 校验 runtime 归属，
	// 而不是 RequesterUserID，使得 requester A 可以为 squad leader B
	// 选择 B 的运行时。
	// 非 squad 场景留空（零值），保持原有 RequesterUserID 校验行为。
	AgentOwnerID        pgtype.UUID
	ExplicitRuntimeID   pgtype.UUID
	AllowExplicitChoice bool
}

type RuntimeResolver struct {
	GetRuntime                 func(context.Context, pgtype.UUID) (db.AgentRuntime, error)
	ListUserCompatibleRuntimes func(context.Context, RuntimeCandidateParams) ([]db.AgentRuntime, error)
}

func NewRuntimeResolver(q *db.Queries) RuntimeResolver {
	return RuntimeResolver{
		GetRuntime: q.GetAgentRuntime,
		ListUserCompatibleRuntimes: func(ctx context.Context, params RuntimeCandidateParams) ([]db.AgentRuntime, error) {
			return q.ListUserCompatibleRuntimes(ctx, db.ListUserCompatibleRuntimesParams{
				WorkspaceID: params.WorkspaceID,
				OwnerID:     params.OwnerID,
				ProfileID:   params.ProfileID,
				Provider:    params.Provider,
			})
		},
	}
}

func (r RuntimeResolver) Resolve(ctx context.Context, input RuntimeResolveInput) (db.AgentRuntime, error) {
	if !input.RequesterUserID.Valid {
		return db.AgentRuntime{}, ErrRuntimeRequesterRequired
	}
	provider := input.Agent.RuntimeProvider
	profileID := input.Agent.RuntimeProfileID
	if !profileID.Valid && (provider == "" || provider == "legacy_local") {
		return db.AgentRuntime{}, ErrNoCompatibleRuntimeForUser
	}

	if input.AllowExplicitChoice && input.ExplicitRuntimeID.Valid {
		rt, err := r.getRuntime(ctx, input.ExplicitRuntimeID)
		if err != nil {
			return db.AgentRuntime{}, err
		}
		if rt.WorkspaceID != input.WorkspaceID {
			return db.AgentRuntime{}, ErrRuntimeNotFound
		}
		// 确定应校验的 owner：AgentOwnerID 优先（squad 场景），
		// 回退到 RequesterUserID（非 squad 场景保持原有行为）。
		expectedOwner := input.RequesterUserID
		if input.AgentOwnerID.Valid {
			expectedOwner = input.AgentOwnerID
		}
		if rt.OwnerID != expectedOwner {
			return db.AgentRuntime{}, ErrRuntimeNotFound
		}
		if rt.RuntimeMode != "local" || rt.Status != "online" {
			return db.AgentRuntime{}, ErrRuntimeUnavailable
		}
		if !runtimeMatchesCapability(rt, provider, profileID) {
			return db.AgentRuntime{}, ErrRuntimeCapabilityMismatch
		}
		return rt, nil
	}

	if r.ListUserCompatibleRuntimes == nil {
		return db.AgentRuntime{}, fmt.Errorf("runtime resolver: list callback missing")
	}
	candidates, err := r.ListUserCompatibleRuntimes(ctx, RuntimeCandidateParams{
		WorkspaceID: input.WorkspaceID,
		OwnerID:     input.RequesterUserID,
		Provider:    provider,
		ProfileID:   profileID,
	})
	if err != nil {
		return db.AgentRuntime{}, fmt.Errorf("list compatible runtimes: %w", err)
	}
	if len(candidates) == 0 {
		return db.AgentRuntime{}, ErrNoCompatibleRuntimeForUser
	}
	return candidates[0], nil
}

func (r RuntimeResolver) getRuntime(ctx context.Context, id pgtype.UUID) (db.AgentRuntime, error) {
	if r.GetRuntime == nil {
		return db.AgentRuntime{}, fmt.Errorf("runtime resolver: get callback missing")
	}
	rt, err := r.GetRuntime(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return db.AgentRuntime{}, ErrRuntimeNotFound
	}
	if err != nil {
		return db.AgentRuntime{}, fmt.Errorf("load runtime: %w", err)
	}
	return rt, nil
}

func runtimeMatchesCapability(rt db.AgentRuntime, provider string, profileID pgtype.UUID) bool {
	if profileID.Valid {
		return rt.ProfileID.Valid && rt.ProfileID == profileID
	}
	return provider != "" && rt.Provider == provider && !rt.ProfileID.Valid
}

func runtimeResolutionUserFacing(err error) bool {
	return errors.Is(err, ErrRuntimeRequesterRequired) ||
		errors.Is(err, ErrRuntimeNotFound) ||
		errors.Is(err, ErrRuntimeNotOwnedByRequester) ||
		errors.Is(err, ErrRuntimeWorkspaceMismatch) ||
		errors.Is(err, ErrRuntimeUnavailable) ||
		errors.Is(err, ErrRuntimeCapabilityMismatch) ||
		errors.Is(err, ErrNoCompatibleRuntimeForUser)
}
