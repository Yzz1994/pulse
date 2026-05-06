package announcements

import "time"

// Announcement 公告条目。
type Announcement struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Content   string    `json:"content"` // Markdown 格式
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Store 公告持久化接口。
type Store interface {
	Create(a *Announcement) error
	Update(a *Announcement) error
	Delete(id string) error
	Get(id string) (*Announcement, error)
	List() ([]Announcement, error)
	// GetActive 返回当前激活的公告，若无则返回 nil, nil。
	GetActive() (*Announcement, error)
	// SetActive 激活指定公告并禁用其余所有公告。
	SetActive(id string) error
	// Disable 禁用指定公告（不删除）。
	Disable(id string) error
}
