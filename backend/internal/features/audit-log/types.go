package auditlog

type ShadowTarget struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	BaseURL   string `json:"baseUrl"`
	TargetURL string `json:"targetUrl"`
}

type ShadowEntry struct {
	ShadowAuditLogID int     `json:"shadowAuditLogId,omitempty"`
	AuditLogID       int     `json:"-"`
	ID               string  `json:"id"`
	Name             string  `json:"name"`
	BaseURL          string  `json:"baseUrl"`
	TargetURL        string  `json:"targetUrl"`
	ResponseStatus   *int    `json:"responseStatus"`
	DurationMS       int     `json:"durationMs"`
	Success          bool    `json:"success"`
	ErrorMessage     *string `json:"errorMessage"`
	RequestHeaders   string  `json:"requestHeaders"`
	RequestBody      string  `json:"requestBody"`
	ResponseHeaders  string  `json:"responseHeaders"`
	ResponseBody     string  `json:"responseBody"`
}

type PostmanExample struct {
	APIMockID    string `json:"apiMockId"`
	Status       int    `json:"status"`
	Name         string `json:"name"`
	ResponseBody string `json:"responseBody"`
}

type CreateInput struct {
	APIMockDBID     *int
	APIMockID       string
	RouteName       string
	Method          string
	RoutePath       string
	TargetURL       string
	ResponseStatus  *int
	DurationMS      int
	Success         bool
	ErrorMessage    *string
	RequestHeaders  string
	RequestBody     string
	ResponseHeaders string
	ResponseBody    string
	ShadowTargets   []ShadowTarget
}

type Record struct {
	InternalID      int              `json:"-"`
	ID              string           `json:"id"`
	APIMockID       *string          `json:"apiMockId"`
	APIMockDBID     *int             `json:"-"`
	RouteName       string           `json:"routeName"`
	Method          string           `json:"method"`
	RoutePath       string           `json:"routePath"`
	TargetURL       string           `json:"targetUrl"`
	ResponseStatus  *int             `json:"responseStatus"`
	DurationMS      int              `json:"durationMs"`
	Success         bool             `json:"success"`
	ErrorMessage    *string          `json:"errorMessage"`
	RequestHeaders  string           `json:"requestHeaders"`
	RequestBody     string           `json:"requestBody"`
	ResponseHeaders string           `json:"responseHeaders"`
	ResponseBody    string           `json:"responseBody"`
	ShadowTargets   []ShadowTarget   `json:"shadowTargets"`
	ShadowEntries   []ShadowEntry    `json:"shadowEntries"`
	PostmanExample  *PostmanExample  `json:"postmanExample"`
	PostmanExamples []PostmanExample `json:"postmanExamples"`
	CreatedAt       string           `json:"createdAt"`
}

type ReplayResult struct {
	TargetURL       string  `json:"targetUrl"`
	ResponseStatus  *int    `json:"responseStatus"`
	DurationMS      int     `json:"durationMs"`
	Success         bool    `json:"success"`
	ErrorMessage    *string `json:"errorMessage"`
	RequestHeaders  string  `json:"requestHeaders"`
	RequestBody     string  `json:"requestBody"`
	ResponseHeaders string  `json:"responseHeaders"`
	ResponseBody    string  `json:"responseBody"`
}

type ReplayRequest struct {
	Method         string `json:"method" binding:"required"`
	TargetURL      string `json:"targetUrl" binding:"required"`
	RequestHeaders string `json:"requestHeaders"`
	RequestBody    string `json:"requestBody"`
}

type ListResult struct {
	Items      []Record `json:"items"`
	Total      int      `json:"total"`
	Page       int      `json:"page"`
	PageSize   int      `json:"pageSize"`
	TotalPages int      `json:"totalPages"`
}
