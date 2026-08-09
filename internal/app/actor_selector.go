package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/morluto/gitcontribute/internal/corpus"
	"github.com/morluto/gitcontribute/internal/mcpcontract"
)

// parsedActorSelector is the executable form of ActorSelector's JSON union.
// Implementations contain exactly one identity, so downstream acquisition does
// not need to keep re-checking the discriminator and mutually exclusive fields.
type parsedActorSelector interface {
	key() string
	resolveLogin(context.Context, *corpus.Corpus) (string, error)
}

type actorLogin string

func (login actorLogin) key() string { return strings.ToLower(string(login)) }
func (login actorLogin) resolveLogin(context.Context, *corpus.Corpus) (string, error) {
	return string(login), nil
}

type actorNodeID string

func (nodeID actorNodeID) key() string { return string(nodeID) }
func (nodeID actorNodeID) resolveLogin(ctx context.Context, c *corpus.Corpus) (string, error) {
	actor, err := c.GetActor(ctx, string(nodeID))
	if err != nil {
		return "", err
	}
	if actor == nil || actor.Login == "" {
		return "", fmt.Errorf("node ID %q is not stored; search or sync by login first", nodeID)
	}
	return actor.Login, nil
}

func parseActorSelectors(inputs []mcpcontract.ActorSelector) ([]parsedActorSelector, []mcpcontract.ActorSelector, error) {
	selectors := make([]parsedActorSelector, len(inputs))
	normalized := make([]mcpcontract.ActorSelector, len(inputs))
	seen := make(map[string]struct{}, len(inputs))
	for i, input := range inputs {
		switch input.Type {
		case "login":
			login := strings.TrimSpace(input.Login)
			if login == "" || input.NodeID != "" {
				return nil, nil, errors.New("login selectors require login and forbid node_id")
			}
			selectors[i] = actorLogin(login)
			normalized[i] = mcpcontract.ActorSelector{Type: "login", Login: login}
		case "node_id":
			nodeID := strings.TrimSpace(input.NodeID)
			if nodeID == "" || input.Login != "" {
				return nil, nil, errors.New("node_id selectors require node_id and forbid login")
			}
			selectors[i] = actorNodeID(nodeID)
			normalized[i] = mcpcontract.ActorSelector{Type: "node_id", NodeID: nodeID}
		default:
			return nil, nil, errors.New("actor selector type must be login or node_id")
		}
		key := selectors[i].key()
		if _, ok := seen[key]; ok {
			return nil, nil, fmt.Errorf("duplicate actor selector %q", key)
		}
		seen[key] = struct{}{}
	}
	return selectors, normalized, nil
}

func storedActorForSelector(ctx context.Context, c *corpus.Corpus, selector parsedActorSelector) (*corpus.Actor, string, error) {
	login, err := selector.resolveLogin(ctx, c)
	if err != nil {
		return nil, "", err
	}
	actor, err := c.GetActor(ctx, login)
	if err != nil {
		return nil, "", err
	}
	if actor == nil {
		return nil, "", fmt.Errorf("actor %q has no stored identity; call github.sync_users first", login)
	}
	return actor, login, nil
}
