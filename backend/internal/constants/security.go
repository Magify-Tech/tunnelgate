package constants

const (
	ControlCharsPattern           = `[\x00-\x08\x0B\x0C\x0E-\x1F\x7F]`
	HTMLTagsPattern               = `<[^>]*>`
	EnvironmentVariableKeyPattern = `^[A-Za-z0-9_.-]+$`
	OpenAPIPathParameterPattern   = `\{([^}/]+)\}`

	URLSchemeHTTP  = "http"
	URLSchemeHTTPS = "https"

	HeaderRuleItemRequest  = "request_header"
	HeaderRuleItemResponse = "response_header"
	HeaderRuleTypeRegex    = "regex"
	HeaderRuleTypeLiteral  = "literal"

	ContentTypeJSON           = "/json"
	ContentTypeStructuredJSON = "+json"
	ContentTypeText           = "text/"
	ContentTypeXML            = "/xml"
	ContentTypeStructuredXML  = "+xml"
	ContentTypeSOAP           = "soap"
	ContentTypeGraphQL        = "graphql"
	ContentTypeFormURLEncoded = "x-www-form-urlencoded"
	ContentTypeMultipartForm  = "multipart/form-data"
	ContentTypeHex            = "hex"
	ContentTypeHTML           = "html"
	ContentTypeYAML           = "yaml"
	ContentTypeYML            = "yml"
	ContentTypeJavaScript     = "javascript"

	PostmanBodyModeURLEncoded = "urlencoded"
	PostmanBodyModeFormData   = "formdata"
	PostmanBodyModeGraphQL    = "graphql"
	PostmanBodyModeRaw        = "raw"

	RequestBodyTypeFormURLEncoded = "x-www-form-urlencoded"
	RequestBodyTypeFormData       = "form-data"
	RequestBodyTypeGraphQL        = "graphql"
	RequestBodyTypeText           = "text"
	RequestBodyTypeXML            = "xml"
	RequestBodyTypeHTML           = "html"
	RequestBodyTypeJavaScript     = "javascript"
	RequestBodyTypeJSON           = "json"
)

var HighRiskSQLPatterns = []string{
	`(?i)(^|[\s'")])(or|and)\s+\d+\s*=\s*\d+($|[\s'"(])`,
	`(?i)\bunion\s+(all\s+)?select\b`,
	`(?i);\s*(select|insert|update|delete|drop|alter|create|truncate|exec|execute|merge)\b`,
	`(--|#|/\*|\*/)`,
	`(?i)\b(sleep\s*\(|benchmark\s*\(|waitfor\s+delay|xp_cmdshell)\b`,
}

var PublicProxyXSSPatterns = []string{
	`(?i)<\s*script\b`,
	`(?i)javascript\s*:`,
	`(?i)\bon[a-z]+\s*=`,
	`(?i)<\s*iframe\b`,
	`(?i)<\s*object\b`,
}

var PublicProxySQLInjectionPatterns = []string{
	`(?i)(?:'|%27|").*\bor\b.*(?:=|like)\s*(?:'|%27|"|[0-9])`,
	`(?i)\bunion\s+select\b`,
	`(?i)\bdrop\s+table\b`,
	`(?i)\binformation_schema\b`,
	`(?i);\s*--`,
	`(?i)\bsleep\s*\(`,
}

var JSONContentTypeMarkers = []string{ContentTypeJSON, ContentTypeStructuredJSON}

var TextBodyContentTypeMarkers = []string{
	ContentTypeJSON,
	ContentTypeStructuredJSON,
	ContentTypeText,
	ContentTypeXML,
	ContentTypeStructuredXML,
	ContentTypeSOAP,
	ContentTypeGraphQL,
	ContentTypeFormURLEncoded,
	ContentTypeMultipartForm,
	ContentTypeHex,
}
