package users

import "github.com/JamesWillian/loobby-api/api-go/internal/httpx"

// Os campos seguem a ordem dos data classes do Kotlin, que e a ordem do JSON, e
// nenhum leva omitempty: o Jackson escreve null.

// meResponse: UserMeResponse.
type meResponse struct {
	ID            string         `json:"id"`
	Username      string         `json:"username"`
	Displayname   *string        `json:"displayname"`
	AvatarURL     *string        `json:"avatarUrl"`
	IsAnonymous   bool           `json:"isAnonymous"`
	Roles         []string       `json:"roles"`
	Email         *string        `json:"email"`
	EmailVerified bool           `json:"emailVerified"`
	CreatedAt     *httpx.Instant `json:"createdAt"`
}

// profileResponse: UserProfileResponse.
type profileResponse struct {
	ID          string         `json:"id"`
	Username    string         `json:"username"`
	Displayname *string        `json:"displayname"`
	AvatarURL   *string        `json:"avatarUrl"`
	CreatedAt   *httpx.Instant `json:"createdAt"`
}

// feedResponse: UserFeedResponse.
type feedResponse struct {
	ID         string  `json:"id"`
	UserID     string  `json:"userId"`
	Name       string  `json:"name"`
	ImageURL   *string `json:"imageUrl"`
	EntryType  string  `json:"entryType"`
	IsFinished bool    `json:"isFinished"`
}

// message: o mapOf("message" to ...) dos endpoints de senha e exclusao.
type message struct {
	Message string `json:"message"`
}
