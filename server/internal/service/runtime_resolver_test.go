package service

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

func TestRuntimeResolverUsesRequesterOwnedRuntime(t *testing.T) {
	workspaceID := testUUID(1)
	requesterID := testUUID(2)
	agentOwnerRuntimeID := testUUID(3)
	requesterRuntimeID := testUUID(4)

	resolver := RuntimeResolver{
		ListUserCompatibleRuntimes: func(ctx context.Context, params RuntimeCandidateParams) ([]db.AgentRuntime, error) {
			if params.WorkspaceID != workspaceID {
				t.Fatalf("workspace mismatch")
			}
			if params.OwnerID != requesterID {
				t.Fatalf("owner mismatch: resolver must search the requester, not the agent owner")
			}
			if params.Provider != "codex" {
				t.Fatalf("provider mismatch: got %q", params.Provider)
			}
			return []db.AgentRuntime{
				{
					ID:          requesterRuntimeID,
					WorkspaceID: workspaceID,
					OwnerID:     requesterID,
					RuntimeMode: "local",
					Provider:    "codex",
					Status:      "online",
				},
			}, nil
		},
		GetRuntime: func(ctx context.Context, id pgtype.UUID) (db.AgentRuntime, error) {
			if id == agentOwnerRuntimeID {
				return db.AgentRuntime{ID: agentOwnerRuntimeID}, nil
			}
			return db.AgentRuntime{}, errors.New("unexpected runtime lookup")
		},
	}

	runtime, err := resolver.Resolve(context.Background(), RuntimeResolveInput{
		WorkspaceID:     workspaceID,
		Agent:           db.Agent{RuntimeID: agentOwnerRuntimeID, RuntimeProvider: "codex"},
		RequesterUserID: requesterID,
	})
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if runtime.ID != requesterRuntimeID {
		t.Fatalf("resolved runtime = %v, want requester runtime %v", runtime.ID, requesterRuntimeID)
	}
}

func TestRuntimeResolverHidesExplicitRuntimeNotOwnedByRequester(t *testing.T) {
	workspaceID := testUUID(1)
	requesterID := testUUID(2)
	otherUserID := testUUID(3)
	otherRuntimeID := testUUID(4)

	resolver := RuntimeResolver{
		GetRuntime: func(ctx context.Context, id pgtype.UUID) (db.AgentRuntime, error) {
			if id != otherRuntimeID {
				t.Fatalf("runtime lookup = %v, want %v", id, otherRuntimeID)
			}
			return db.AgentRuntime{
				ID:          otherRuntimeID,
				WorkspaceID: workspaceID,
				OwnerID:     otherUserID,
				RuntimeMode: "local",
				Provider:    "codex",
				Status:      "online",
			}, nil
		},
	}

	_, err := resolver.Resolve(context.Background(), RuntimeResolveInput{
		WorkspaceID:         workspaceID,
		Agent:               db.Agent{RuntimeProvider: "codex"},
		RequesterUserID:     requesterID,
		ExplicitRuntimeID:   otherRuntimeID,
		AllowExplicitChoice: true,
	})
	if !errors.Is(err, ErrRuntimeNotFound) {
		t.Fatalf("Resolve error = %v, want ErrRuntimeNotFound", err)
	}
}

func TestRuntimeResolverRejectsExplicitNonLocalRuntime(t *testing.T) {
	workspaceID := testUUID(1)
	requesterID := testUUID(2)
	cloudRuntimeID := testUUID(3)

	resolver := RuntimeResolver{
		GetRuntime: func(ctx context.Context, id pgtype.UUID) (db.AgentRuntime, error) {
			if id != cloudRuntimeID {
				t.Fatalf("runtime lookup = %v, want %v", id, cloudRuntimeID)
			}
			return db.AgentRuntime{
				ID:          cloudRuntimeID,
				WorkspaceID: workspaceID,
				OwnerID:     requesterID,
				RuntimeMode: "cloud",
				Provider:    "codex",
				Status:      "online",
			}, nil
		},
	}

	_, err := resolver.Resolve(context.Background(), RuntimeResolveInput{
		WorkspaceID:         workspaceID,
		Agent:               db.Agent{RuntimeProvider: "codex"},
		RequesterUserID:     requesterID,
		ExplicitRuntimeID:   cloudRuntimeID,
		AllowExplicitChoice: true,
	})
	if !errors.Is(err, ErrRuntimeUnavailable) {
		t.Fatalf("Resolve error = %v, want ErrRuntimeUnavailable", err)
	}
}

func TestRuntimeResolverRejectsMissingRequesterWithoutFallback(t *testing.T) {
	resolver := RuntimeResolver{
		GetRuntime: func(ctx context.Context, id pgtype.UUID) (db.AgentRuntime, error) {
			t.Fatal("resolver must not load an explicit runtime without a requester")
			return db.AgentRuntime{}, nil
		},
		ListUserCompatibleRuntimes: func(ctx context.Context, params RuntimeCandidateParams) ([]db.AgentRuntime, error) {
			t.Fatal("resolver must not list runtime candidates without a requester")
			return nil, nil
		},
	}

	_, err := resolver.Resolve(context.Background(), RuntimeResolveInput{
		WorkspaceID: testUUID(1),
		Agent: db.Agent{
			RuntimeProvider: "codex",
			RuntimeID:       testUUID(3),
		},
		ExplicitRuntimeID:   testUUID(4),
		AllowExplicitChoice: true,
	})
	if !errors.Is(err, ErrRuntimeRequesterRequired) {
		t.Fatalf("Resolve error = %v, want ErrRuntimeRequesterRequired", err)
	}
}

func TestRuntimeResolverRejectsAgentWithoutCapabilityBeforeListing(t *testing.T) {
	resolver := RuntimeResolver{
		ListUserCompatibleRuntimes: func(ctx context.Context, params RuntimeCandidateParams) ([]db.AgentRuntime, error) {
			t.Fatal("resolver must not list runtime candidates without agent capability")
			return nil, nil
		},
	}

	_, err := resolver.Resolve(context.Background(), RuntimeResolveInput{
		WorkspaceID:     testUUID(1),
		Agent:           db.Agent{RuntimeProvider: "legacy_local", RuntimeID: testUUID(3)},
		RequesterUserID: testUUID(2),
	})
	if !errors.Is(err, ErrNoCompatibleRuntimeForUser) {
		t.Fatalf("Resolve error = %v, want ErrNoCompatibleRuntimeForUser", err)
	}
}

func TestRuntimeResolutionUserFacingClassification(t *testing.T) {
	userFacing := []error{
		ErrRuntimeRequesterRequired,
		ErrRuntimeNotFound,
		ErrRuntimeNotOwnedByRequester,
		ErrRuntimeWorkspaceMismatch,
		ErrRuntimeUnavailable,
		ErrRuntimeCapabilityMismatch,
		ErrNoCompatibleRuntimeForUser,
	}
	for _, err := range userFacing {
		if !runtimeResolutionUserFacing(err) {
			t.Fatalf("runtimeResolutionUserFacing(%v) = false, want true", err)
		}
	}

	infraErr := errors.New("database timeout")
	if runtimeResolutionUserFacing(infraErr) {
		t.Fatalf("runtimeResolutionUserFacing(%v) = true, want false", infraErr)
	}
}

func TestQuickCreateRuntimeVersionGate(t *testing.T) {
	okRuntime := db.AgentRuntime{
		ID:       testUUID(1),
		Metadata: []byte(`{"cli_version":"v0.2.21"}`),
	}
	if err := checkQuickCreateRuntimeVersion(okRuntime); err != nil {
		t.Fatalf("checkQuickCreateRuntimeVersion returned error for current runtime: %v", err)
	}

	oldRuntime := db.AgentRuntime{
		ID:       testUUID(2),
		Metadata: []byte(`{"cli_version":"v0.2.20"}`),
	}
	err := checkQuickCreateRuntimeVersion(oldRuntime)
	var versionErr *QuickCreateDaemonVersionError
	if !errors.As(err, &versionErr) {
		t.Fatalf("checkQuickCreateRuntimeVersion error = %v, want QuickCreateDaemonVersionError", err)
	}
	if versionErr.RuntimeID != oldRuntime.ID {
		t.Fatalf("RuntimeID = %v, want %v", versionErr.RuntimeID, oldRuntime.ID)
	}
	if versionErr.CurrentVersion != "v0.2.20" {
		t.Fatalf("CurrentVersion = %q, want v0.2.20", versionErr.CurrentVersion)
	}
	if versionErr.MinVersion == "" {
		t.Fatal("MinVersion is empty")
	}
}
func TestRuntimeResolverExplicitChoiceUsesAgentOwnerForSquad(t *testing.T) {
	workspaceID := testUUID(1)
	requesterID := testUUID(2)     // 用户 A（issue 创建者）
	agentOwnerID := testUUID(3)    // 用户 B（squad leader agent 的 owner）
	squadLeaderRuntimeID := testUUID(4) // 用户 B 的运行时

	resolver := RuntimeResolver{
		GetRuntime: func(ctx context.Context, id pgtype.UUID) (db.AgentRuntime, error) {
			if id != squadLeaderRuntimeID {
				t.Fatalf("runtime lookup = %v, want %v", id, squadLeaderRuntimeID)
			}
			return db.AgentRuntime{
				ID:          squadLeaderRuntimeID,
				WorkspaceID: workspaceID,
				OwnerID:     agentOwnerID, // 运行时属于 B
				RuntimeMode: "local",
				Provider:    "codex",
				Status:      "online",
			}, nil
		},
	}

	// 用户 A 创建 issue 分配给 squad leader B，选择 B 的运行时
	// AgentOwnerID=B 应当允许该选择，尽管 RequesterUserID=A ≠ B
	runtime, err := resolver.Resolve(context.Background(), RuntimeResolveInput{
		WorkspaceID:         workspaceID,
		Agent:               db.Agent{RuntimeProvider: "codex"},
		RequesterUserID:     requesterID,
		AgentOwnerID:        agentOwnerID, // squad leader agent 的 owner = B
		ExplicitRuntimeID:   squadLeaderRuntimeID,
		AllowExplicitChoice: true,
	})
	if err != nil {
		t.Fatalf("Resolve returned error for squad leader runtime selection: %v", err)
	}
	if runtime.ID != squadLeaderRuntimeID {
		t.Fatalf("resolved runtime = %v, want squad leader runtime %v", runtime.ID, squadLeaderRuntimeID)
	}
}

func TestRuntimeResolverRejectsExplicitChoiceWhenAgentOwnerMismatch(t *testing.T) {
	workspaceID := testUUID(1)
	requesterID := testUUID(2)
	agentOwnerID := testUUID(3)       // agent owner = B
	otherUserID := testUUID(4)        // 用户 C
	otherRuntimeID := testUUID(5)     // C 的运行时

	resolver := RuntimeResolver{
		GetRuntime: func(ctx context.Context, id pgtype.UUID) (db.AgentRuntime, error) {
			if id != otherRuntimeID {
				t.Fatalf("runtime lookup = %v, want %v", id, otherRuntimeID)
			}
			return db.AgentRuntime{
				ID:          otherRuntimeID,
				WorkspaceID: workspaceID,
				OwnerID:     otherUserID, // 运行时属于 C
				RuntimeMode: "local",
				Provider:    "codex",
				Status:      "online",
			}, nil
		},
	}

	// 设置 AgentOwnerID=B，但运行时属于 C（既不是 A 也不是 B）
	_, err := resolver.Resolve(context.Background(), RuntimeResolveInput{
		WorkspaceID:         workspaceID,
		Agent:               db.Agent{RuntimeProvider: "codex"},
		RequesterUserID:     requesterID,
		AgentOwnerID:        agentOwnerID,
		ExplicitRuntimeID:   otherRuntimeID,
		AllowExplicitChoice: true,
	})
	if !errors.Is(err, ErrRuntimeNotFound) {
		t.Fatalf("Resolve error = %v, want ErrRuntimeNotFound (runtime belongs to neither requester nor agent owner)", err)
	}
}

func TestRuntimeResolverExplicitChoiceFallsBackToRequesterWhenNoAgentOwner(t *testing.T) {
	workspaceID := testUUID(1)
	requesterID := testUUID(2)
	requesterRuntimeID := testUUID(3)

	resolver := RuntimeResolver{
		GetRuntime: func(ctx context.Context, id pgtype.UUID) (db.AgentRuntime, error) {
			if id != requesterRuntimeID {
				t.Fatalf("runtime lookup = %v, want %v", id, requesterRuntimeID)
			}
			return db.AgentRuntime{
				ID:          requesterRuntimeID,
				WorkspaceID: workspaceID,
				OwnerID:     requesterID, // 运行时属于 requester
				RuntimeMode: "local",
				Provider:    "codex",
				Status:      "online",
			}, nil
		},
	}

	// 非 squad 场景：AgentOwnerID 未设置，应使用 RequesterUserID 校验
	runtime, err := resolver.Resolve(context.Background(), RuntimeResolveInput{
		WorkspaceID:         workspaceID,
		Agent:               db.Agent{RuntimeProvider: "codex"},
		RequesterUserID:     requesterID,
		// AgentOwnerID: 留空（零值）
		ExplicitRuntimeID:   requesterRuntimeID,
		AllowExplicitChoice: true,
	})
	if err != nil {
		t.Fatalf("Resolve returned error for requester's own runtime: %v", err)
	}
	if runtime.ID != requesterRuntimeID {
		t.Fatalf("resolved runtime = %v, want requester runtime %v", runtime.ID, requesterRuntimeID)
	}
}
