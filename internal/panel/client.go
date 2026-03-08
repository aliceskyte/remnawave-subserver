package panel

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

// maxResponseBytes limits the size of panel API responses to 10 MB.
const maxResponseBytes = 10 << 20

type Client struct {
	baseURL    string
	token      string
	timeout    time.Duration
	httpClient *http.Client
	cache      *Cache
}

type Error struct {
	Code any
}

func (e Error) Error() string {
	return fmt.Sprint(e.Code)
}

func Code(err error) (any, bool) {
	var panelErr Error
	if errors.As(err, &panelErr) {
		return panelErr.Code, true
	}
	return nil, false
}

type UserInfo struct {
	VlessUUID  string
	Username   string
	PanelUUID  string
	SquadUUIDs []string
}

type InternalSquad struct {
	UUID         string `json:"uuid"`
	Name         string `json:"name"`
	ViewPosition int    `json:"viewPosition"`
}

func NewClient(baseURL, token string, timeout time.Duration, cache *Cache) *Client {
	cleanURL := strings.TrimRight(baseURL, "/")
	return &Client{
		baseURL: cleanURL,
		token:   token,
		timeout: timeout,
		httpClient: &http.Client{
			Timeout: timeout,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		},
		cache: cache,
	}
}

func (c *Client) WithToken(token string) *Client {
	if c == nil {
		return NewClient("", token, 0, nil)
	}
	return NewClient(c.baseURL, token, c.timeout, nil)
}

func (c *Client) UserByShortUUID(ctx context.Context, shortUUID string) (UserInfo, error) {
	response, err := c.get(ctx, "/api/users/by-short-uuid/"+escapedPathSegment(shortUUID), true)
	if err != nil {
		return UserInfo{}, err
	}
	vlessUUID, ok := response["vlessUuid"].(string)
	username, ok2 := response["username"].(string)
	panelUUID, _ := response["uuid"].(string)
	if panelUUID == "" {
		if fallback, ok := response["id"].(string); ok {
			panelUUID = fallback
		}
	}
	if !ok || !ok2 || vlessUUID == "" || username == "" {
		return UserInfo{}, Error{Code: "missing_user_fields"}
	}
	squadUUIDs := extractActiveSquads(response)
	return UserInfo{
		VlessUUID:  vlessUUID,
		Username:   username,
		PanelUUID:  panelUUID,
		SquadUUIDs: squadUUIDs,
	}, nil
}

func (c *Client) SubscriptionRawByShortUUID(ctx context.Context, shortUUID string) (map[string]any, error) {
	return c.get(ctx, "/api/subscriptions/by-short-uuid/"+escapedPathSegment(shortUUID)+"/raw", true)
}

func (c *Client) SubscriptionRawByShortUUIDFresh(ctx context.Context, shortUUID string) (map[string]any, error) {
	return c.get(ctx, "/api/subscriptions/by-short-uuid/"+escapedPathSegment(shortUUID)+"/raw", false)
}

func (c *Client) SubscriptionByShortUUID(ctx context.Context, shortUUID string) (map[string]any, error) {
	return c.get(ctx, "/api/subscriptions/by-short-uuid/"+escapedPathSegment(shortUUID), true)
}

func (c *Client) RemnawaveSettings(ctx context.Context) (map[string]any, error) {
	return c.get(ctx, "/api/remnawave-settings", false)
}

func (c *Client) InternalSquads(ctx context.Context) ([]InternalSquad, error) {
	response, err := c.get(ctx, "/api/internal-squads", false)
	if err != nil {
		return nil, err
	}
	rawSquads, ok := response["internalSquads"].([]any)
	if !ok {
		return []InternalSquad{}, nil
	}
	squads := make([]InternalSquad, 0, len(rawSquads))
	for _, entry := range rawSquads {
		item, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		uuid, _ := item["uuid"].(string)
		name, _ := item["name"].(string)
		viewPosition := toInt(item["viewPosition"])
		if strings.TrimSpace(uuid) == "" {
			continue
		}
		squads = append(squads, InternalSquad{
			UUID:         uuid,
			Name:         name,
			ViewPosition: viewPosition,
		})
	}
	sort.SliceStable(squads, func(i, j int) bool {
		if squads[i].ViewPosition == squads[j].ViewPosition {
			return squads[i].Name < squads[j].Name
		}
		return squads[i].ViewPosition < squads[j].ViewPosition
	})
	return squads, nil
}

func (c *Client) get(ctx context.Context, path string, useCache bool) (map[string]any, error) {
	if c == nil {
		return nil, Error{Code: "panel_unreachable"}
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	if useCache && c.cache != nil {
		if cached, ok := c.cache.Get(path); ok {
			return cached, nil
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return nil, Error{Code: "panel_invalid_response"}
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, Error{Code: "panel_unreachable"}
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return nil, Error{Code: resp.StatusCode}
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if err != nil {
		return nil, Error{Code: "panel_invalid_response"}
	}

	var payload map[string]any
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(&payload); err != nil {
		return nil, Error{Code: "panel_invalid_response"}
	}

	response, ok := payload["response"].(map[string]any)
	if !ok {
		return nil, Error{Code: "panel_invalid_payload"}
	}

	if useCache && c.cache != nil {
		c.cache.Set(path, response)
	}
	return response, nil
}

func escapedPathSegment(value string) string {
	return url.PathEscape(strings.TrimSpace(value))
}

func extractActiveSquads(response map[string]any) []string {
	raw, ok := response["activeInternalSquads"].([]any)
	if !ok {
		return nil
	}
	squads := make([]string, 0, len(raw))
	for _, entry := range raw {
		item, ok := entry.(map[string]any)
		if !ok {
			continue
		}
		uuid, _ := item["uuid"].(string)
		uuid = strings.TrimSpace(uuid)
		if uuid == "" {
			continue
		}
		squads = append(squads, uuid)
	}
	return squads
}

func toInt(value any) int {
	switch v := value.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case json.Number:
		if parsed, err := v.Int64(); err == nil {
			return int(parsed)
		}
		if parsed, err := v.Float64(); err == nil {
			return int(parsed)
		}
	case string:
		if parsed, err := strconv.Atoi(v); err == nil {
			return parsed
		}
	}
	return 0
}
