package mocks

import "devstreamlinebot/interfaces"

// MockVKMessage represents a mock VK Teams message.
type MockVKMessage struct {
	ChatID    string
	Text      string
	ParseMode string
	Sent      bool
	SendErr   error
}

// Send simulates sending a message.
func (m *MockVKMessage) Send() error {
	m.Sent = true
	return m.SendErr
}

// MockVKBot is a mock implementation of VK Teams bot.
// Implements interfaces.VKBot.
type MockVKBot struct {
	Messages []*MockVKMessage
}

// Ensure MockVKBot implements interfaces.VKBot.
var _ interfaces.VKBot = (*MockVKBot)(nil)

// NewMockVKBot creates a new MockVKBot.
func NewMockVKBot() *MockVKBot {
	return &MockVKBot{
		Messages: make([]*MockVKMessage, 0),
	}
}

// NewTextMessage creates a mock text message and tracks it.
// Returns interfaces.VKBotMessage to satisfy the interface.
func (m *MockVKBot) NewTextMessage(chatID string, text string) interfaces.VKBotMessage {
	msg := &MockVKMessage{
		ChatID: chatID,
		Text:   text,
	}
	m.Messages = append(m.Messages, msg)
	return msg
}

func (m *MockVKBot) NewHTMLMessage(chatID string, text string) interfaces.VKBotMessage {
	msg := &MockVKMessage{
		ChatID:    chatID,
		Text:      text,
		ParseMode: "HTML",
	}
	m.Messages = append(m.Messages, msg)
	return msg
}

// GetSentMessages returns all messages that were sent.
func (m *MockVKBot) GetSentMessages() []*MockVKMessage {
	var sent []*MockVKMessage
	for _, msg := range m.Messages {
		if msg.Sent {
			sent = append(sent, msg)
		}
	}
	return sent
}

// Reset clears all tracked messages.
func (m *MockVKBot) Reset() {
	m.Messages = make([]*MockVKMessage, 0)
}
