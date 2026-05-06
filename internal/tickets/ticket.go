package tickets

import "time"

// TicketStatus 工单状态。
type TicketStatus string

const (
	StatusOpen    TicketStatus = "open"    // 待处理
	StatusReplied TicketStatus = "replied" // 已回复
	StatusClosed  TicketStatus = "closed"  // 已关闭
)

// Ticket 工单主体。
type Ticket struct {
	ID        string       `json:"id"`
	UserID    string       `json:"user_id"`
	Username  string       `json:"username"`
	Title     string       `json:"title"`
	Status    TicketStatus `json:"status"`
	CreatedAt time.Time    `json:"created_at"`
	UpdatedAt time.Time    `json:"updated_at"`
}

// Message 工单消息（用户提交或管理员回复）。
type Message struct {
	ID        string    `json:"id"`
	TicketID  string    `json:"ticket_id"`
	Content   string    `json:"content"` // Markdown 格式
	IsAdmin   bool      `json:"is_admin"`
	CreatedAt time.Time `json:"created_at"`
}

// Image 工单图片元数据。
type Image struct {
	ID         string    `json:"id"`
	TicketID   string    `json:"ticket_id"`
	Filename   string    `json:"filename"`    // 原始文件名
	StoredName string    `json:"stored_name"` // 存储文件名（uuid.ext）
	Size       int64     `json:"size"`
	CreatedAt  time.Time `json:"created_at"`
}

// Store 工单持久化接口。
type Store interface {
	CreateTicket(t *Ticket) error
	GetTicket(id string) (*Ticket, error)
	ListTickets() ([]Ticket, error)
	ListTicketsByUser(userID string) ([]Ticket, error)
	UpdateTicketStatus(id string, status TicketStatus) error

	AddMessage(m *Message) error
	ListMessages(ticketID string) ([]Message, error)

	AddImage(img *Image) error
	ListImages(ticketID string) ([]Image, error)
}
