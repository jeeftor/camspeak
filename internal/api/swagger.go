package api

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

// swaggerUIHTML serves Swagger UI from a CDN, pointing at our /api/openapi.json spec.
const swaggerUIHTML = `<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8" />
  <meta name="viewport" content="width=device-width, initial-scale=1" />
  <title>camspeak — Swagger UI</title>
  <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5.18.2/swagger-ui.css" />
  <style>
    body { margin: 0; }
    .topbar { display: none; }
  </style>
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://unpkg.com/swagger-ui-dist@5.18.2/swagger-ui-bundle.js"></script>
  <script>
    window.onload = () => {
      window.ui = SwaggerUIBundle({
        url: '/api/openapi.json',
        dom_id: '#swagger-ui',
        deepLinking: true,
        presets: [SwaggerUIBundle.presets.apis],
        layout: 'BaseLayout',
        tryItOutEnabled: true,
        requestInterceptor: (req) => {
          // Use relative URLs so it works regardless of host/domain
          return req;
        },
      });
    };
  </script>
</body>
</html>`

// SwaggerUI handles GET /swagger — serves the Swagger UI HTML page.
func SwaggerUI(c echo.Context) error {
	return c.HTML(http.StatusOK, swaggerUIHTML)
}

// OpenAPISpec handles GET /api/openapi.json — serves the OpenAPI 3.0 spec.
func OpenAPISpec(c echo.Context) error {
	return c.JSONBlob(http.StatusOK, []byte(openAPISpec))
}
