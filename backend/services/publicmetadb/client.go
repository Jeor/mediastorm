package publicmetadb

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const DefaultBaseURL = "https://publicmetadb.com/api"

type Client struct {
	BaseURL string
	HTTP    *http.Client
}

type List struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

type Item struct {
	MediaType string `json:"media_type"`
	TMDBID    int64  `json:"tmdb_id"`
}

type page[T any] struct {
	Items      []T `json:"items"`
	Page       int `json:"page"`
	TotalPages int `json:"totalPages"`
}

func (c *Client) get(ctx context.Context, key, path string, out any) error {
	base := c.BaseURL
	if base == "" {
		base = DefaultBaseURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(base, "/")+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("PublicMetaDB returned HTTP %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func fetchAll[T any](ctx context.Context, c *Client, key, path string) ([]T, error) {
	var all []T
	for n := 1; n <= 100; n++ {
		separator := "?"
		if strings.Contains(path, "?") {
			separator = "&"
		}
		var result page[T]
		err := c.get(ctx, key, path+separator+"page="+strconv.Itoa(n)+"&perPage=100", &result)
		if err != nil {
			return nil, err
		}
		all = append(all, result.Items...)
		if result.TotalPages == 0 || n >= result.TotalPages {
			return all, nil
		}
	}
	return nil, fmt.Errorf("PublicMetaDB pagination exceeded 100 pages")
}

func (c *Client) Lists(ctx context.Context, key string) ([]List, error) {
	return fetchAll[List](ctx, c, key, "/external/lists")
}

func (c *Client) Items(ctx context.Context, key, listID string) ([]Item, error) {
	return fetchAll[Item](ctx, c, key, "/external/lists/"+url.PathEscape(listID)+"/items")
}
