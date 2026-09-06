package canvas

import "testing"

func TestAPIErrorIsShortAndFriendly(t *testing.T) {
	tests := []struct {
		name string
		err  *APIError
		want string
	}{
		{
			name: "canvas 401",
			err: &APIError{
				Status: 401,
				Path:   "https://canvas.nus.edu.sg/api/v1/users/self",
				Body:   `{"status":"unauthenticated","errors":[{"message":"user authorization required"}]}`,
			},
			want: "canvas: GET /users/self -> 401 unauthenticated (user authorization required)",
		},
		{
			name: "errors keyed by field",
			err: &APIError{
				Status: 400,
				Path:   "https://canvas.nus.edu.sg/api/v1/courses/1/files",
				Body:   `{"errors":{"base":[{"message":"invalid include"}]}}`,
			},
			want: "canvas: GET /courses/1/files -> 400 (invalid include)",
		},
		{
			name: "no body falls back to the status text",
			err:  &APIError{Status: 403, Path: "https://canvas.nus.edu.sg/api/v1/courses/2/folders"},
			want: "canvas: GET /courses/2/folders -> 403 forbidden",
		},
		{
			name: "non-JSON body is collapsed to one line",
			err: &APIError{
				Status: 502,
				Path:   "https://canvas.nus.edu.sg/api/v1/users/self",
				Body:   "<html>\n  <body>bad gateway</body>\n</html>",
			},
			want: "canvas: GET /users/self -> 502 (<html> <body>bad gateway</body> </html>)",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.want {
				t.Errorf("Error()\n got %q\nwant %q", got, tt.want)
			}
		})
	}
}

func TestAPIErrorNeverGrowsUnbounded(t *testing.T) {
	long := make([]byte, 0, 4000)
	for len(long) < 3000 {
		long = append(long, 'x')
	}
	e := &APIError{Status: 500, Path: "https://x/api/v1/a", Body: string(long)}
	if len(e.Error()) > 220 {
		t.Errorf("error message is %d chars, want it capped", len(e.Error()))
	}
}
