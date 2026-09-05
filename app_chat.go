package main

// In-app Claude chat, over a synced file, a library paper, or nothing at all.
// Turns run through the same single-slot Study job queue (Kind "chat"), so a
// chat never competes with an overview or a quiz for the subscription.
// See docs/CONTRACT_PAPERS.md.

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"nussync/internal/store"
	"nussync/internal/study"
)

// StartChat opens a session bound to a file (fileID), a library paper
// (paperID), or neither. Nothing is sent to Claude until SendChat.
func (a *App) StartChat(fileID int, paperID string, model string) (ChatSession, error) {
	if err := a.papersInit(); err != nil {
		return ChatSession{}, err
	}
	if err := a.studyInit(); err != nil {
		return ChatSession{}, err
	}
	now := nowRFC3339()
	s := store.ChatSession{
		ID:        fmt.Sprintf("chat_%d", time.Now().UnixNano()),
		FileID:    fileID,
		PaperID:   strings.TrimSpace(paperID),
		Model:     study.NormalizeModel(model),
		Title:     "New chat",
		CreatedAt: now,
		UpdatedAt: now,
	}
	if name, _, _ := a.chatContext(s); name != "" {
		s.Title = name
	}
	if err := a.st.PutChatSession(s); err != nil {
		return ChatSession{}, err
	}
	return toChatSession(s), nil
}

// SendChat queues one turn and returns its job id. The reply streams back as
// `chat:delta` events and finishes with `chat:done`.
func (a *App) SendChat(sessionID string, message string) (string, error) {
	if err := a.papersInit(); err != nil {
		return "", err
	}
	if err := a.studyInit(); err != nil {
		return "", err
	}
	if strings.TrimSpace(message) == "" {
		return "", errors.New("empty message")
	}
	s, ok, err := a.st.ChatSessionByID(sessionID)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", fmt.Errorf("no chat session %s", sessionID)
	}

	if _, err := a.st.InsertChatMessage(store.ChatMessage{
		SessionID: s.ID, Role: "user", Text: strings.TrimSpace(message),
		CreatedAt: nowRFC3339(),
	}); err != nil {
		return "", err
	}
	if s.Title == "" || s.Title == "New chat" {
		s.Title = study.ChatTitle(message)
		s.UpdatedAt = nowRFC3339()
		_ = a.st.PutChatSession(s)
	}

	fileIDs := []int(nil)
	if s.FileID != 0 {
		fileIDs = []int{s.FileID}
	}
	j := a.newStudyJob("chat", fileIDs, s.Model)
	err = a.enqueueStudy(j, func(ctx context.Context, a *App, jobID string) (float64, error) {
		cost, err := a.runChatTurn(ctx, jobID, s.ID, message)
		errText := ""
		if err != nil {
			errText = err.Error()
		}
		a.emit("chat:done", ChatDone{SessionID: s.ID, JobID: jobID, Error: errText})
		return cost, err
	})
	if err != nil {
		return j.ID, err
	}
	return j.ID, nil
}

// runChatTurn executes one turn: first turn plain, later turns with --resume,
// falling back to a transcript replay when the CLI has lost the session.
func (a *App) runChatTurn(ctx context.Context, jobID, sessionID, message string) (float64, error) {
	s, ok, err := a.st.ChatSessionByID(sessionID)
	if err != nil {
		return 0, err
	}
	if !ok {
		return 0, fmt.Errorf("no chat session %s", sessionID)
	}

	name, path, fileID := a.chatContext(s)
	needRead := path != "" && study.ClaudeCanRead(path)
	dir := filepath.Dir(path)
	if path == "" {
		dir = a.settings().SyncDir
	}
	var dirs []string
	if needRead {
		dirs = []string{dir}
	}

	first := strings.TrimSpace(s.ClaudeSessionID) == ""
	prompt := study.ChatPrompt(path, name, message, first)
	if first && path != "" && !needRead && fileID != 0 {
		// Office formats: Claude Code cannot open them, so inline the text
		// NUSSync already extracted for FTS.
		if txt, _ := a.st.StudyFileText(fileID); strings.TrimSpace(txt) != "" {
			prompt = "Context document: " + name + ". Its extracted text follows " +
				"between the markers; answer from it.\n<<<BEGIN>>>\n" +
				study.ClampText(txt) + "\n<<<END>>>\n\n" + prompt
		}
	}

	var sb strings.Builder
	onText := func(t string) {
		sb.WriteString(t)
		a.emit("chat:delta", ChatDelta{SessionID: s.ID, Text: t})
	}
	onProgress := func(t string) { a.setStudyProgress(jobID, t) }

	o := study.ChatOptions(s.Model, s.ClaudeSessionID, dir, dirs, needRead)
	res, err := study.RunChat(ctx, prompt, o, onText, onProgress)
	cost := res.CostUSD

	if !first && (err != nil || res.IsError) && ctx.Err() == nil {
		// --resume can fail when the CLI no longer has that session on disk;
		// replay the transcript in a fresh run instead.
		history, _ := a.st.ChatMessages(s.ID)
		turns := make([]study.ChatTurn, 0, len(history))
		for _, m := range history {
			if m.Role == "user" && strings.TrimSpace(m.Text) == strings.TrimSpace(message) &&
				m.ID == history[len(history)-1].ID {
				continue // the message we are about to ask
			}
			turns = append(turns, study.ChatTurn{Role: m.Role, Text: m.Text})
		}
		sb.Reset()
		fresh := study.ChatOptions(s.Model, "", dir, dirs, needRead)
		res, err = study.RunChat(ctx, study.ReplayPrompt(path, name, turns, message),
			fresh, onText, onProgress)
		cost += res.CostUSD
	}
	if err != nil {
		return cost, err
	}
	if res.IsError {
		return cost, errors.New(strings.TrimSpace(res.Text))
	}

	text := strings.TrimSpace(res.Text)
	if text == "" {
		text = strings.TrimSpace(sb.String())
	}
	if text == "" {
		return cost, errors.New("Claude returned an empty reply")
	}
	if _, err := a.st.InsertChatMessage(store.ChatMessage{
		SessionID: s.ID, Role: "assistant", Text: text, CreatedAt: nowRFC3339(),
	}); err != nil {
		return cost, err
	}
	if res.SessionID != "" && res.SessionID != s.ClaudeSessionID {
		_ = a.st.SetChatClaudeSession(s.ID, res.SessionID)
	}
	return cost, nil
}

// chatContext resolves a session's context into (display name, absolute path,
// file id). Everything is "" / 0 for a context-free chat.
func (a *App) chatContext(s store.ChatSession) (name, path string, fileID int) {
	if s.FileID != 0 {
		if f, ok, err := a.st.FileByID(s.FileID); err == nil && ok {
			if st, e := os.Stat(f.AbsPath); e == nil && !st.IsDir() {
				return f.Name, f.AbsPath, f.ID
			}
			return f.Name, "", f.ID
		}
	}
	if strings.TrimSpace(s.PaperID) != "" {
		if lp, ok, err := a.st.LibraryPaperByID(s.PaperID); err == nil && ok {
			if lp.LocalPath != "" {
				if st, e := os.Stat(lp.LocalPath); e == nil && !st.IsDir() {
					return lp.Title, lp.LocalPath, lp.FileID
				}
			}
			return lp.Title, "", lp.FileID
		}
	}
	return "", "", 0
}

// GetChats lists sessions for a context, newest first. fileID 0 with an empty
// paperID lists every session.
func (a *App) GetChats(fileID int, paperID string) ([]ChatSession, error) {
	if err := a.papersInit(); err != nil {
		return nil, err
	}
	rows, err := a.st.ChatSessions(fileID, paperID)
	if err != nil {
		return nil, err
	}
	out := make([]ChatSession, 0, len(rows))
	for _, r := range rows {
		out = append(out, toChatSession(r))
	}
	return out, nil
}

// GetChatMessages lists a session's turns, oldest first.
func (a *App) GetChatMessages(sessionID string) ([]ChatMessage, error) {
	if err := a.papersInit(); err != nil {
		return nil, err
	}
	rows, err := a.st.ChatMessages(sessionID)
	if err != nil {
		return nil, err
	}
	out := make([]ChatMessage, 0, len(rows))
	for _, r := range rows {
		out = append(out, ChatMessage{ID: r.ID, SessionID: r.SessionID,
			Role: r.Role, Text: r.Text, CreatedAt: r.CreatedAt})
	}
	return out, nil
}

// DeleteChat removes a session and its messages.
func (a *App) DeleteChat(sessionID string) error {
	if err := a.papersInit(); err != nil {
		return err
	}
	return a.st.DeleteChatSession(sessionID)
}

func toChatSession(c store.ChatSession) ChatSession {
	return ChatSession{
		ID: c.ID, FileID: c.FileID, PaperID: c.PaperID, Title: c.Title,
		ClaudeSessionID: c.ClaudeSessionID, Model: c.Model,
		CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt,
	}
}
