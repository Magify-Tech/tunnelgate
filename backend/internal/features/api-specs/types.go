package mockapi

type ResponseExample struct {
	Status          int    `json:"status" binding:"required"`
	Name            string `json:"name"`
	Body            string `json:"body"`
	BodyType        string `json:"bodyType"`
	ResponseHeaders string `json:"responseHeaders"`
}

type RequestBodyTypes map[string]string

type Definition struct {
	CollectionName    string
	RouteName         string
	PostmanFolderPath []string
	Method            string
	RoutePath         string
	ResponseStatus    int
	ResponseBody      string
	ResponseBodyType  string
	RequestHeaders    string
	ResponseHeaders   string
	ResponseExamples  []ResponseExample
	RequestBodyRaw    string
	RequestBodyType   string
	RequestBodyKeys   []string
	RequestBodyTypes  RequestBodyTypes
	RequestParamKeys  []string
	RequestParamTypes RequestBodyTypes
}

type ManualSpecInput struct {
	CollectionName    string            `json:"collectionName"`
	RouteName         string            `json:"routeName"`
	PostmanFolderPath []string          `json:"postmanFolderPath"`
	Method            string            `json:"method" binding:"required"`
	RoutePath         string            `json:"routePath" binding:"required"`
	MockEnabled       bool              `json:"mockEnabled"`
	ProxyEnabled      bool              `json:"proxyEnabled"`
	ResponseStatus    int               `json:"responseStatus"`
	ResponseExamples  []ResponseExample `json:"responseExamples"`
	RequestHeaders    string            `json:"requestHeaders"`
	RequestBodyRaw    string            `json:"requestBodyRaw"`
	RequestBodyType   string            `json:"requestBodyType"`
	RequestBodyKeys   []string          `json:"requestBodyKeys"`
	RequestBodyTypes  RequestBodyTypes  `json:"requestBodyTypes"`
	RequestParamKeys  []string          `json:"requestParamKeys"`
	RequestParamTypes RequestBodyTypes  `json:"requestParamTypes"`
}

type StoredAPI struct {
	InternalID        int
	ID                string
	CollectionName    string
	RouteName         string
	PostmanFolderPath string
	Method            string
	RoutePath         string
	MockEnabled       bool
	ProxyEnabled      bool
	ResponseStatus    int
	ResponseBody      string
	ResponseBodyType  string
	RequestHeaders    string
	ResponseHeaders   string
	RequestBodyKeys   string
	RequestBodyTypes  string
	RequestBodyRaw    string
	RequestBodyType   string
	RequestParamKeys  string
	RequestParamTypes string
	UpdatedAt         string
}

type APIRecord struct {
	InternalID           int               `json:"-"`
	ID                   string            `json:"id"`
	CollectionName       string            `json:"collectionName"`
	RouteName            string            `json:"routeName"`
	PostmanFolderPath    []string          `json:"postmanFolderPath"`
	Method               string            `json:"method"`
	RoutePath            string            `json:"routePath"`
	ResolvedRoutePath    string            `json:"resolvedRoutePath"`
	MockEnabled          bool              `json:"mockEnabled"`
	ProxyEnabled         bool              `json:"proxyEnabled"`
	ResponseStatus       int               `json:"responseStatus"`
	ResponseBody         string            `json:"responseBody"`
	ResponseBodyType     string            `json:"responseBodyType"`
	RequestHeaders       string            `json:"requestHeaders"`
	ResponseHeaders      string            `json:"responseHeaders"`
	ResponseExamples     []ResponseExample `json:"responseExamples"`
	RequestBodyRaw       string            `json:"requestBodyRaw"`
	RequestBodyType      string            `json:"requestBodyType"`
	ExpectedRequestKeys  []string          `json:"expectedRequestKeys"`
	ExpectedRequestTypes RequestBodyTypes  `json:"expectedRequestTypes"`
	ExpectedParamKeys    []string          `json:"expectedParamKeys"`
	ExpectedParamTypes   RequestBodyTypes  `json:"expectedParamTypes"`
	UpdatedAt            string            `json:"updatedAt"`
}

type ListResult struct {
	Items      []APIRecord `json:"items"`
	Total      int         `json:"total"`
	Page       int         `json:"page"`
	PageSize   int         `json:"pageSize"`
	TotalPages int         `json:"totalPages"`
}

type DirectoryChild struct {
	Name     string   `json:"name"`
	Path     []string `json:"path"`
	APICount int      `json:"apiCount"`
}

type DirectoryResult struct {
	Mode          string           `json:"mode"`
	Path          []string         `json:"path"`
	Children      []DirectoryChild `json:"children"`
	APIs          []APIRecord      `json:"apis"`
	TotalAPICount int              `json:"totalApiCount"`
}

type CollectionsResult struct {
	Collections []string `json:"collections"`
}

type VersionCreateInput struct {
	Message string `json:"message"`
}

type ToggleInput struct {
	Enabled *bool `json:"enabled" binding:"required"`
}

type SelectResponseInput struct {
	Status int `json:"status" binding:"required"`
}

type ProjectVersion struct {
	ID        string `json:"id"`
	Message   string `json:"message"`
	APICount  int    `json:"apiCount"`
	CreatedAt string `json:"createdAt"`
}

type ProjectVersionRecord struct {
	ProjectVersion
	Snapshot []APIRecord `json:"snapshot"`
}
