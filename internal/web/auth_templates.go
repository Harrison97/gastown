package web

import "html/template"

// loginTemplate is the HTML template for the login page.
var loginTemplate = template.Must(template.New("login").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Login - Gas Town</title>
    <style>
        :root {
            --bg-dark: #0f1419;
            --bg-card: #1a1f26;
            --bg-card-hover: #242b33;
            --text-primary: #e6e1cf;
            --text-secondary: #6c7680;
            --text-muted: #4a5159;
            --border: #2d363f;
            --border-accent: #3d4752;
            --green: #c2d94c;
            --yellow: #ffb454;
            --red: #f07178;
            --blue: #59c2ff;
        }

        * {
            box-sizing: border-box;
            margin: 0;
            padding: 0;
        }

        body {
            font-family: 'SF Mono', 'Menlo', 'Monaco', 'Consolas', monospace;
            background: var(--bg-dark);
            color: var(--text-primary);
            min-height: 100vh;
            display: flex;
            align-items: center;
            justify-content: center;
            font-size: 14px;
            line-height: 1.5;
        }

        .login-container {
            width: 100%;
            max-width: 400px;
            padding: 20px;
        }

        .login-card {
            background: var(--bg-card);
            border: 1px solid var(--border);
            border-radius: 8px;
            padding: 32px;
        }

        .login-header {
            text-align: center;
            margin-bottom: 32px;
        }

        .login-header h1 {
            font-size: 1.5rem;
            font-weight: 600;
            color: var(--text-primary);
            margin-bottom: 8px;
        }

        .login-header .subtitle {
            color: var(--text-secondary);
            font-size: 0.875rem;
        }

        .form-group {
            margin-bottom: 20px;
        }

        .form-group label {
            display: block;
            color: var(--text-secondary);
            font-size: 0.75rem;
            text-transform: uppercase;
            letter-spacing: 0.05em;
            margin-bottom: 8px;
        }

        .form-group input {
            width: 100%;
            padding: 12px 16px;
            background: var(--bg-dark);
            border: 1px solid var(--border);
            border-radius: 6px;
            color: var(--text-primary);
            font-family: inherit;
            font-size: 1rem;
            transition: border-color 0.2s;
        }

        .form-group input:focus {
            outline: none;
            border-color: var(--blue);
        }

        .form-group input::placeholder {
            color: var(--text-muted);
        }

        .error-message {
            background: rgba(240, 113, 120, 0.1);
            border: 1px solid var(--red);
            border-radius: 6px;
            padding: 12px 16px;
            color: var(--red);
            font-size: 0.875rem;
            margin-bottom: 20px;
        }

        .submit-btn {
            width: 100%;
            padding: 14px 20px;
            background: var(--green);
            border: none;
            border-radius: 6px;
            color: var(--bg-dark);
            font-family: inherit;
            font-size: 1rem;
            font-weight: 600;
            cursor: pointer;
            transition: opacity 0.2s;
        }

        .submit-btn:hover {
            opacity: 0.9;
        }

        .submit-btn:active {
            opacity: 0.8;
        }

        .login-footer {
            text-align: center;
            margin-top: 24px;
            color: var(--text-muted);
            font-size: 0.75rem;
        }
    </style>
</head>
<body>
    <div class="login-container">
        <div class="login-card">
            <div class="login-header">
                <h1>Gas Town</h1>
                <div class="subtitle">Dashboard Access</div>
            </div>

            {{if .Error}}
            <div class="error-message">{{.Error}}</div>
            {{end}}

            <form method="POST" action="/login">
                <div class="form-group">
                    <label for="password">Password</label>
                    <input type="password" id="password" name="password"
                           placeholder="Enter your password" required autofocus>
                </div>

                <button type="submit" class="submit-btn">Sign In</button>
            </form>

            <div class="login-footer">
                Gas Town Control Center
            </div>
        </div>
    </div>
</body>
</html>`))

// setupTemplate is the HTML template for initial password setup.
var setupTemplate = template.Must(template.New("setup").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Setup - Gas Town</title>
    <style>
        :root {
            --bg-dark: #0f1419;
            --bg-card: #1a1f26;
            --bg-card-hover: #242b33;
            --text-primary: #e6e1cf;
            --text-secondary: #6c7680;
            --text-muted: #4a5159;
            --border: #2d363f;
            --border-accent: #3d4752;
            --green: #c2d94c;
            --yellow: #ffb454;
            --red: #f07178;
            --blue: #59c2ff;
        }

        * {
            box-sizing: border-box;
            margin: 0;
            padding: 0;
        }

        body {
            font-family: 'SF Mono', 'Menlo', 'Monaco', 'Consolas', monospace;
            background: var(--bg-dark);
            color: var(--text-primary);
            min-height: 100vh;
            display: flex;
            align-items: center;
            justify-content: center;
            font-size: 14px;
            line-height: 1.5;
        }

        .setup-container {
            width: 100%;
            max-width: 400px;
            padding: 20px;
        }

        .setup-card {
            background: var(--bg-card);
            border: 1px solid var(--border);
            border-radius: 8px;
            padding: 32px;
        }

        .setup-header {
            text-align: center;
            margin-bottom: 32px;
        }

        .setup-header h1 {
            font-size: 1.5rem;
            font-weight: 600;
            color: var(--text-primary);
            margin-bottom: 8px;
        }

        .setup-header .subtitle {
            color: var(--text-secondary);
            font-size: 0.875rem;
        }

        .info-box {
            background: rgba(89, 194, 255, 0.1);
            border: 1px solid var(--blue);
            border-radius: 6px;
            padding: 12px 16px;
            color: var(--blue);
            font-size: 0.875rem;
            margin-bottom: 24px;
        }

        .form-group {
            margin-bottom: 20px;
        }

        .form-group label {
            display: block;
            color: var(--text-secondary);
            font-size: 0.75rem;
            text-transform: uppercase;
            letter-spacing: 0.05em;
            margin-bottom: 8px;
        }

        .form-group input {
            width: 100%;
            padding: 12px 16px;
            background: var(--bg-dark);
            border: 1px solid var(--border);
            border-radius: 6px;
            color: var(--text-primary);
            font-family: inherit;
            font-size: 1rem;
            transition: border-color 0.2s;
        }

        .form-group input:focus {
            outline: none;
            border-color: var(--blue);
        }

        .form-group input::placeholder {
            color: var(--text-muted);
        }

        .form-group .hint {
            color: var(--text-muted);
            font-size: 0.75rem;
            margin-top: 6px;
        }

        .error-message {
            background: rgba(240, 113, 120, 0.1);
            border: 1px solid var(--red);
            border-radius: 6px;
            padding: 12px 16px;
            color: var(--red);
            font-size: 0.875rem;
            margin-bottom: 20px;
        }

        .submit-btn {
            width: 100%;
            padding: 14px 20px;
            background: var(--green);
            border: none;
            border-radius: 6px;
            color: var(--bg-dark);
            font-family: inherit;
            font-size: 1rem;
            font-weight: 600;
            cursor: pointer;
            transition: opacity 0.2s;
        }

        .submit-btn:hover {
            opacity: 0.9;
        }

        .submit-btn:active {
            opacity: 0.8;
        }

        .setup-footer {
            text-align: center;
            margin-top: 24px;
            color: var(--text-muted);
            font-size: 0.75rem;
        }
    </style>
</head>
<body>
    <div class="setup-container">
        <div class="setup-card">
            <div class="setup-header">
                <h1>Gas Town Setup</h1>
                <div class="subtitle">Create Dashboard Password</div>
            </div>

            <div class="info-box">
                Set a password to protect your dashboard. This password will be
                required for all future access.
            </div>

            {{if .Error}}
            <div class="error-message">{{.Error}}</div>
            {{end}}

            <form method="POST" action="/setup">
                <div class="form-group">
                    <label for="password">Password</label>
                    <input type="password" id="password" name="password"
                           placeholder="Choose a password" required autofocus>
                    <div class="hint">Minimum 8 characters</div>
                </div>

                <div class="form-group">
                    <label for="confirm">Confirm Password</label>
                    <input type="password" id="confirm" name="confirm"
                           placeholder="Confirm your password" required>
                </div>

                <button type="submit" class="submit-btn">Create Password</button>
            </form>

            <div class="setup-footer">
                Gas Town Control Center
            </div>
        </div>
    </div>
</body>
</html>`))
