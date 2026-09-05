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

type rawUpdate struct {
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

// Update is one incoming message, flattened to what NUSSync needs.
type Update struct {
	ID     int64  // update_id
	ChatID string // decimal chat id, "" when the update carried no message
	Text   string
}

// GetUpdates long-polls the Bot API. offset is the next update id to receive
// (0 = whatever Telegram still has queued); timeoutSec is the server-side long
// poll, which must stay below the HTTP client timeout (90s).
//
// Telegram allows exactly ONE getUpdates consumer per bot: a second concurrent
// caller makes both of them lose updates at random. Everything in NUSSync goes
// through notify.Bot, which owns the single poll loop; AwaitStart and
// FindExistingChat below are only for the headless CLI, where no bot runs.
func (c *Client) GetUpdates(ctx context.Context, offset int64, timeoutSec int) ([]Update, error) {
	v := url.Values{}
	v.Set("timeout", strconv.Itoa(timeoutSec))
	v.Set("allowed_updates", `["message"]`)
	if offset != 0 {
		v.Set("offset", strconv.FormatInt(offset, 10))
	}
	var ups []rawUpdate
	if err := c.call(ctx, "getUpdates", v, &ups); err != nil {
		return nil, err
	}
	out := make([]Update, 0, len(ups))
	for _, u := range ups {
		up := Update{ID: u.UpdateID}
		if u.Message != nil {
			up.Text = u.Message.Text
			if u.Message.Chat.ID != 0 {
				up.ChatID = strconv.FormatInt(u.Message.Chat.ID, 10)
			}
		}
		out = append(out, up)
	}
	return out, nil
}

// AwaitStart long-polls getUpdates until someone sends /start to the bot, and
// returns their chat id. It also matches a /start already sitting in the queue.
// Returns an error if the timeout elapses first.
//
// Only for the headless CLI (--pair). In the GUI, notify.Bot is the single
// getUpdates consumer and App.PairTelegram waits on it instead.
func (c *Client) AwaitStart(ctx context.Context, timeout time.Duration) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	notPaired := fmt.Errorf("telegram: no /start received (send /start to @%s)", BotUsername)
	var offset int64
	deadline := time.Now().Add(timeout)
	for {
		if err := ctx.Err(); err != nil {
			return "", notPaired
		}
		remaining := int(time.Until(deadline).Seconds())
		if remaining < 1 {
			return "", notPaired
		}
		if remaining > 25 {
			remaining = 25
		}

		ups, err := c.GetUpdates(ctx, offset, remaining)
		if err != nil {
			if ctx.Err() != nil {
				return "", notPaired
			}
			return "", err
		}
		for _, u := range ups {
			if u.ID >= offset {
				offset = u.ID + 1
			}
			if u.ChatID == "" {
				continue
			}
			if strings.HasPrefix(strings.TrimSpace(u.Text), "/start") {
				return u.ChatID, nil
			}
		}
	}
}

// FindExistingChat does a single non-blocking getUpdates poll looking for a
// /start (or any message) already in the queue. Returns "" if none.
func (c *Client) FindExistingChat(ctx context.Context) (string, error) {
	ups, err := c.GetUpdates(ctx, 0, 0)
	if err != nil {
		return "", err
	}
	for i := len(ups) - 1; i >= 0; i-- {
		if ups[i].ChatID != "" {
			return ups[i].ChatID, nil
		}
	}
	return "", nil
}

// EscapeHTML escapes text for Telegram's HTML parse mode.
func EscapeHTML(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")
	return r.Replace(s)
}
