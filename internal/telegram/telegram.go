// Package telegram is a minimal Bot API client for notifications and pairing.
package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// BotUsername is the bot users pair with.
const BotUsername = "nuscanvassync_bot"

// Client talks to api.telegram.org.
type Client struct {
	Token string
	HTTP  *http.Client
}

// New builds a client for a bot token.
func New(token string) *Client {
	return &Client{Token: token, HTTP: &http.Client{Timeout: 90 * time.Second}}
}

type apiResponse struct {
	OK          bool            `json:"ok"`
	Result      json.RawMessage `json:"result"`
	Description string          `json:"description"`
	ErrorCode   int             `json:"error_code"`
}

func (c *Client) call(ctx context.Context, method string, params url.Values, out any) error {
	if strings.TrimSpace(c.Token) == "" {
		return fmt.Errorf("telegram: no bot token configured")
	}
	endpoint := "https://api.telegram.org/bot" + c.Token + "/" + method
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint,
		strings.NewReader(params.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return err
	}
	var ar apiResponse
	if err := json.Unmarshal(body, &ar); err != nil {
		return fmt.Errorf("telegram: %s: bad response (HTTP %d)", method, resp.StatusCode)
	}
	if !ar.OK {
		return fmt.Errorf("telegram: %s: %s", method, ar.Description)
	}
	if out != nil {
		return json.Unmarshal(ar.Result, out)
	}
	return nil
}

// Bot describes the authenticated bot.
type Bot struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
	Name     string `json:"first_name"`
}

// GetMe verifies the token and returns the bot identity.
func (c *Client) GetMe(ctx context.Context) (Bot, error) {
	var b Bot
	err := c.call(ctx, "getMe", url.Values{}, &b)
	return b, err
}

// SendMessage posts an HTML-formatted message to a chat.
func (c *Client) SendMessage(ctx context.Context, chatID, text string) error {
	if strings.TrimSpace(chatID) == "" {
		return fmt.Errorf("telegram: not paired (no chat id)")
	}
	v := url.Values{}
	v.Set("chat_id", chatID)
	v.Set("text", text)
	v.Set("parse_mode", "HTML")
	v.Set("disable_web_page_preview", "true")
	return c.call(ctx, "sendMessage", v, nil)
}

type update struct {
	UpdateID int64 `json:"update_id"`
	Message  *struct {
		Text string `json:"text"`
		Chat struct {
			ID       int64  `json:"id"`
			Username string `json:"username"`
			Type     string `json:"type"`
		} `json:"chat"`
	} `json:"message"`
}

// AwaitStart long-polls getUpdates until someone sends /start to the bot, and
// returns their chat id. It also matches a /start already sitting in the queue.
// Returns an error if the timeout elapses first.
func (c *Client) AwaitStart(ctx context.Context, timeout time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var offset int64
	deadline := time.Now().Add(timeout)
	for {
		if err := ctx.Err(); err != nil {
			return "", fmt.Errorf("telegram: no /start received (send /start to @%s)", BotUsername)
		}
		remaining := int(time.Until(deadline).Seconds())
		if remaining < 1 {
			return "", fmt.Errorf("telegram: no /start received (send /start to @%s)", BotUsername)
		}
		if remaining > 25 {
			remaining = 25
		}

		v := url.Values{}
		v.Set("timeout", strconv.Itoa(remaining))
		v.Set("allowed_updates", `["message"]`)
		if offset != 0 {
			v.Set("offset", strconv.FormatInt(offset, 10))
		}

		var ups []update
		if err := c.call(ctx, "getUpdates", v, &ups); err != nil {
			if ctx.Err() != nil {
				return "", fmt.Errorf("telegram: no /start received (send /start to @%s)", BotUsername)
			}
			return "", err
		}
		for _, u := range ups {
			if u.UpdateID >= offset {
				offset = u.UpdateID + 1
			}
			if u.Message == nil {
				continue
			}
			if strings.HasPrefix(strings.TrimSpace(u.Message.Text), "/start") {
				return strconv.FormatInt(u.Message.Chat.ID, 10), nil
			}
		}
	}
}

// FindExistingChat does a single non-blocking getUpdates poll looking for a
// /start (or any message) already in the queue. Returns "" if none.
func (c *Client) FindExistingChat(ctx context.Context) (string, error) {
	v := url.Values{}
	v.Set("timeout", "0")
	v.Set("allowed_updates", `["message"]`)
	var ups []update
	if err := c.call(ctx, "getUpdates", v, &ups); err != nil {
		return "", err
	}
	for i := len(ups) - 1; i >= 0; i-- {
		if ups[i].Message != nil && ups[i].Message.Chat.ID != 0 {
			return strconv.FormatInt(ups[i].Message.Chat.ID, 10), nil
		}
	}
	return "", nil
}

// EscapeHTML escapes text for Telegram's HTML parse mode.
func EscapeHTML(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	return r.Replace(s)
}
