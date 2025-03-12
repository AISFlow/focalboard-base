package api

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strings"

	"github.com/gorilla/mux"
	"github.com/mattermost/focalboard/server/model"
	"github.com/mattermost/focalboard/server/services/audit"
	"github.com/mattermost/focalboard/server/services/auth"
	"github.com/mattermost/focalboard/server/utils"

	"github.com/mattermost/mattermost-server/v6/shared/mlog"
)

func (a *API) registerAuthRoutes(r *mux.Router) {
	// personal-server specific routes. These are not needed in plugin mode.
	if !a.isPlugin {
		r.HandleFunc("/login", a.handleLogin).Methods("POST")
		r.HandleFunc("/logout", a.sessionRequired(a.handleLogout)).Methods("POST")
		r.HandleFunc("/register", a.handleRegister).Methods("POST")
		r.HandleFunc("/registerorfetch", a.handleRegisterOrFetch).Methods("POST")
		r.HandleFunc("/teams/{teamID}/regenerate_signup_token", a.sessionRequired(a.handlePostTeamRegenerateSignupToken)).Methods("POST")
		r.HandleFunc("/users/{userID}/changepassword", a.sessionRequired(a.handleChangePassword)).Methods("POST")
		r.HandleFunc("/users/{userID}/changeemail", a.sessionRequired(a.handleChangeEmail)).Methods("POST")
		r.HandleFunc("/users/{userID}/changeusername", a.sessionRequired(a.handleChangeUsername)).Methods("POST")
		r.HandleFunc("/users/{userID}", a.sessionRequired(a.handleDelete)).Methods("DELETE")
	}
}

func (a *API) handleLogin(w http.ResponseWriter, r *http.Request) {
	// swagger:operation POST /login login
	//
	// Login user
	//
	// ---
	// produces:
	// - application/json
	// parameters:
	// - name: body
	//   in: body
	//   description: Login request
	//   required: true
	//   schema:
	//     "$ref": "#/definitions/LoginRequest"
	// responses:
	//   '200':
	//     description: success
	//     schema:
	//       "$ref": "#/definitions/LoginResponse"
	//   '401':
	//     description: invalid login
	//     schema:
	//       "$ref": "#/definitions/ErrorResponse"
	//   '500':
	//     description: internal error
	//     schema:
	//       "$ref": "#/definitions/ErrorResponse"
	if a.MattermostAuth {
		a.errorResponse(w, r, model.NewErrNotImplemented("not permitted in plugin mode"))
		return
	}

	if len(a.singleUserToken) > 0 {
		// Not permitted in single-user mode
		a.errorResponse(w, r, model.NewErrUnauthorized("not permitted in single-user mode"))
		return
	}

	requestBody, err := io.ReadAll(r.Body)
	if err != nil {
		a.errorResponse(w, r, err)
		return
	}

	var loginData model.LoginRequest
	err = json.Unmarshal(requestBody, &loginData)
	if err != nil {
		a.errorResponse(w, r, err)
		return
	}

	auditRec := a.makeAuditRecord(r, "login", audit.Fail)
	defer a.audit.LogRecord(audit.LevelAuth, auditRec)
	auditRec.AddMeta("username", loginData.Username)
	auditRec.AddMeta("type", loginData.Type)

	if loginData.Type == "normal" {
		token, err := a.app.Login(loginData.Username, loginData.Email, loginData.Password, loginData.MfaToken)
		if err != nil {
			a.errorResponse(w, r, model.NewErrUnauthorized("incorrect login"))
			return
		}
		json, err := json.Marshal(model.LoginResponse{Token: token})
		if err != nil {
			a.errorResponse(w, r, err)
			return
		}

		jsonBytesResponse(w, http.StatusOK, json)
		auditRec.Success()
		return
	}

	a.errorResponse(w, r, model.NewErrBadRequest("invalid login type"))
}

func (a *API) handleLogout(w http.ResponseWriter, r *http.Request) {
	// swagger:operation POST /logout logout
	//
	// Logout user
	//
	// ---
	// produces:
	// - application/json
	// security:
	// - BearerAuth: []
	// responses:
	//   '200':
	//     description: success
	//   '500':
	//     description: internal error
	//     schema:
	//       "$ref": "#/definitions/ErrorResponse"
	if a.MattermostAuth {
		a.errorResponse(w, r, model.NewErrNotImplemented("not permitted in plugin mode"))
		return
	}

	if len(a.singleUserToken) > 0 {
		// Not permitted in single-user mode
		a.errorResponse(w, r, model.NewErrUnauthorized("not permitted in single-user mode"))
		return
	}

	ctx := r.Context()

	session := ctx.Value(sessionContextKey).(*model.Session)

	auditRec := a.makeAuditRecord(r, "logout", audit.Fail)
	defer a.audit.LogRecord(audit.LevelAuth, auditRec)
	auditRec.AddMeta("userID", session.UserID)

	if err := a.app.Logout(session.ID); err != nil {
		a.errorResponse(w, r, model.NewErrUnauthorized("incorrect logout"))
		return
	}

	auditRec.AddMeta("sessionID", session.ID)

	jsonStringResponse(w, http.StatusOK, "{}")
	auditRec.Success()
}

func (a *API) handleRegister(w http.ResponseWriter, r *http.Request) {
	// swagger:operation POST /register register
	//
	// Register new user
	//
	// ---
	// produces:
	// - application/json
	// parameters:
	// - name: body
	//   in: body
	//   description: Register request
	//   required: true
	//   schema:
	//     "$ref": "#/definitions/RegisterRequest"
	// responses:
	//   '200':
	//     description: success
	//   '401':
	//     description: invalid registration token
	//   '500':
	//     description: internal error
	//     schema:
	//       "$ref": "#/definitions/ErrorResponse"
	if a.MattermostAuth {
		a.errorResponse(w, r, model.NewErrNotImplemented("not permitted in plugin mode"))
		return
	}

	if len(a.singleUserToken) > 0 {
		// Not permitted in single-user mode
		a.errorResponse(w, r, model.NewErrUnauthorized("not permitted in single-user mode"))
		return
	}

	requestBody, err := io.ReadAll(r.Body)
	if err != nil {
		a.errorResponse(w, r, err)
		return
	}

	var registerData model.RegisterRequest
	err = json.Unmarshal(requestBody, &registerData)
	if err != nil {
		a.errorResponse(w, r, err)
		return
	}
	registerData.Email = strings.TrimSpace(registerData.Email)
	registerData.Username = strings.TrimSpace(registerData.Username)

	// Validate token
	if len(registerData.Token) > 0 {
		team, err2 := a.app.GetRootTeam()
		if err2 != nil {
			a.errorResponse(w, r, err2)
			return
		}

		if registerData.Token != team.SignupToken {
			a.errorResponse(w, r, model.NewErrUnauthorized("invalid token"))
			return
		}
	} else {
		// No signup token, check if no active users
		userCount, err2 := a.app.GetRegisteredUserCount()
		if err2 != nil {
			a.errorResponse(w, r, err2)
			return
		}
		if userCount > 0 {
			a.errorResponse(w, r, model.NewErrUnauthorized("no sign-up token and user(s) already exist"))
			return
		}
	}

	if err = registerData.IsValid(); err != nil {
		a.errorResponse(w, r, err)
		return
	}

	auditRec := a.makeAuditRecord(r, "register", audit.Fail)
	defer a.audit.LogRecord(audit.LevelAuth, auditRec)
	auditRec.AddMeta("username", registerData.Username)

	err = a.app.RegisterUser(registerData.Username, registerData.Email, registerData.Password)
	if err != nil {
		a.errorResponse(w, r, model.NewErrBadRequest(err.Error()))
		return
	}

	jsonStringResponse(w, http.StatusOK, "{}")
	auditRec.Success()
}

func (a *API) handleRegisterOrFetch(w http.ResponseWriter, r *http.Request) {
	// swagger:operation POST /registerorfetch registerorfetch
	//
	// Register new user or fetch existing
	//
	// ---
	// produces:
	// - application/json
	// parameters:
	// - name: body
	//   in: body
	//   description: Register request
	//   required: true
	//   schema:
	//     "$ref": "#/definitions/RegisterRequest"
	// responses:
	//   '200':
	//     description: success
	//   '401':
	//     description: invalid registration token
	//   '500':
	//     description: internal error
	if a.MattermostAuth {
		a.errorResponse(w, r, model.NewErrNotImplemented("not permitted in plugin mode"))
		return
	}

	if len(a.singleUserToken) > 0 {
		// Not permitted in single-user mode
		a.errorResponse(w, r, model.NewErrUnauthorized("not permitted in single-user mode"))
		return
	}

	requestBody, err := io.ReadAll(r.Body)
	if err != nil {
		a.errorResponse(w, r, err)
		return
	}

	var registerData model.RegisterRequest
	err = json.Unmarshal(requestBody, &registerData)
	if err != nil {
		a.errorResponse(w, r, err)
		return
	}
	registerData.Email = strings.TrimSpace(registerData.Email)
	registerData.Username = strings.TrimSpace(registerData.Username)

	// Validate token
	if len(registerData.Token) > 0 {
		team, err2 := a.app.GetRootTeam()
		if err2 != nil {
			a.errorResponse(w, r, err2)
			return
		}

		if registerData.Token != team.SignupToken {
			a.errorResponse(w, r, model.NewErrUnauthorized("invalid token"))
			return
		}
	} else {
		// No signup token, check if no active users
		userCount, err2 := a.app.GetRegisteredUserCount()
		if err2 != nil {
			a.errorResponse(w, r, err2)
			return
		}
		if userCount > 0 {
			a.errorResponse(w, r, model.NewErrUnauthorized("no sign-up token and user(s) already exist"))
			return
		}
	}

	if err = registerData.IsValid(); err != nil {
		a.errorResponse(w, r, err)
		return
	}

	auditRec := a.makeAuditRecord(r, "registerorfetch", audit.Fail)
	defer a.audit.LogRecord(audit.LevelAuth, auditRec)
	auditRec.AddMeta("username", registerData.Username)

	userId, err := a.app.RegisterOrFetchUser(registerData.Username, registerData.Email, registerData.Password)
	if err != nil && userId == "" {
		a.errorResponse(w, r, model.NewErrBadRequest(err.Error()))
		return
	}

	if err != nil {
		// means the user was already there
		// TODO alexeyqu improve semantics here
		jsonStringResponse(w, http.StatusOK, "{\"userId\": \""+userId+"\", \"isNew\": false}")
	} else {
		// means we've created a new one
		jsonStringResponse(w, http.StatusOK, "{\"userId\": \""+userId+"\", \"isNew\": true}")
	}

	auditRec.Success()
}

func (a *API) handleChangePassword(w http.ResponseWriter, r *http.Request) {
	// swagger:operation POST /users/{userID}/changepassword changePassword
	//
	// Change a user's password
	//
	// ---
	// produces:
	// - application/json
	// parameters:
	// - name: userID
	//   in: path
	//   description: User ID
	//   required: true
	//   type: string
	// - name: body
	//   in: body
	//   description: Change password request
	//   required: true
	//   schema:
	//     "$ref": "#/definitions/ChangePasswordRequest"
	// security:
	// - BearerAuth: []
	// responses:
	//   '200':
	//     description: success
	//   '400':
	//     description: invalid request
	//     schema:
	//       "$ref": "#/definitions/ErrorResponse"
	//   '500':
	//     description: internal error
	//     schema:
	//       "$ref": "#/definitions/ErrorResponse"
	if a.MattermostAuth {
		a.errorResponse(w, r, model.NewErrNotImplemented("not permitted in plugin mode"))
		return
	}

	if len(a.singleUserToken) > 0 {
		// Not permitted in single-user mode
		a.errorResponse(w, r, model.NewErrUnauthorized("not permitted in single-user mode"))
		return
	}

	vars := mux.Vars(r)
	userID := vars["userID"]

	requestBody, err := io.ReadAll(r.Body)
	if err != nil {
		a.errorResponse(w, r, err)
		return
	}

	var requestData model.ChangePasswordRequest
	if err = json.Unmarshal(requestBody, &requestData); err != nil {
		a.errorResponse(w, r, err)
		return
	}

	if err = requestData.IsValid(); err != nil {
		a.errorResponse(w, r, err)
		return
	}

	auditRec := a.makeAuditRecord(r, "changePassword", audit.Fail)
	defer a.audit.LogRecord(audit.LevelAuth, auditRec)

	if err = a.app.ChangePassword(userID, requestData.OldPassword, requestData.NewPassword); err != nil {
		a.errorResponse(w, r, model.NewErrBadRequest(err.Error()))
		return
	}

	jsonStringResponse(w, http.StatusOK, "{}")
	auditRec.Success()
}

func (a *API) handleChangeEmail(w http.ResponseWriter, r *http.Request) {
	// swagger:operation POST /users/{userID}/changeemail changeEmail
	//
	// Change a user's email
	//
	// ---
	// produces:
	// - application/json
	// parameters:
	// - name: userID
	//   in: path
	//   description: User ID
	//   required: true
	//   type: string
	// - name: body
	//   in: body
	//   description: Change email request
	//   required: true
	//   schema:
	//     "$ref": "#/definitions/ChangeEmailRequest"
	// security:
	// - BearerAuth: []
	// responses:
	//   '200':
	//     description: success
	//   '400':
	//     description: invalid request
	//     schema:
	//       "$ref": "#/definitions/ErrorResponse"
	//   '500':
	//     description: internal error
	//     schema:
	//       "$ref": "#/definitions/ErrorResponse"
	if a.MattermostAuth {
		a.errorResponse(w, r, model.NewErrNotImplemented("not permitted in plugin mode"))
		return
	}

	if len(a.singleUserToken) > 0 {
		// Not permitted in single-user mode
		a.errorResponse(w, r, model.NewErrUnauthorized("not permitted in single-user mode"))
		return
	}

	vars := mux.Vars(r)
	userID := vars["userID"]

	requestBody, err := io.ReadAll(r.Body)
	if err != nil {
		a.errorResponse(w, r, err)
		return
	}

	var requestData model.ChangeEmailRequest
	if err = json.Unmarshal(requestBody, &requestData); err != nil {
		a.errorResponse(w, r, err)
		return
	}

	if err = requestData.IsValid(); err != nil {
		a.errorResponse(w, r, err)
		return
	}

	auditRec := a.makeAuditRecord(r, "changeEmail", audit.Fail)
	defer a.audit.LogRecord(audit.LevelAuth, auditRec)

	if err = a.app.ChangeEmail(userID, requestData.Password, requestData.NewEmail); err != nil {
		a.errorResponse(w, r, model.NewErrBadRequest(err.Error()))
		return
	}

	jsonStringResponse(w, http.StatusOK, "{}")
	auditRec.Success()
}

func (a *API) handleChangeUsername(w http.ResponseWriter, r *http.Request) {
	// swagger:operation POST /users/{userID}/changeusername changeUsername
	//
	// Change username
	//
	// ---
	// produces:
	// - application/json
	// parameters:
	// - name: userID
	//   in: path
	//   description: User ID
	//   required: true
	//   type: string
	// - name: body
	//   in: body
	//   description: Change username request
	//   required: true
	//   schema:
	//     "$ref": "#/definitions/ChangeUsernameRequest"
	// security:
	// - BearerAuth: []
	// responses:
	//   '200':
	//     description: success
	//   '400':
	//     description: invalid request
	//     schema:
	//       "$ref": "#/definitions/ErrorResponse"
	//   '500':
	//     description: internal error
	//     schema:
	//       "$ref": "#/definitions/ErrorResponse"
	if a.MattermostAuth {
		a.errorResponse(w, r, model.NewErrNotImplemented("not permitted in plugin mode"))
		return
	}

	if len(a.singleUserToken) > 0 {
		// Not permitted in single-user mode
		a.errorResponse(w, r, model.NewErrUnauthorized("not permitted in single-user mode"))
		return
	}

	vars := mux.Vars(r)
	userID := vars["userID"]

	requestBody, err := io.ReadAll(r.Body)
	if err != nil {
		a.errorResponse(w, r, err)
		return
	}

	var requestData model.ChangeUsernameRequest
	if err = json.Unmarshal(requestBody, &requestData); err != nil {
		a.errorResponse(w, r, err)
		return
	}

	if err = requestData.IsValid(); err != nil {
		a.errorResponse(w, r, err)
		return
	}

	auditRec := a.makeAuditRecord(r, "changeUsername", audit.Fail)
	defer a.audit.LogRecord(audit.LevelAuth, auditRec)

	if err = a.app.ChangeUsername(userID, requestData.Password, requestData.NewUsername); err != nil {
		a.errorResponse(w, r, model.NewErrBadRequest(err.Error()))
		return
	}

	jsonStringResponse(w, http.StatusOK, "{}")
	auditRec.Success()
}

func (a *API) handleDelete(w http.ResponseWriter, r *http.Request) {
	// swagger:operation DELETE /users/{userID} delete
	//
	// Delete user
	//
	// ---
	// produces:
	// - application/json
	// parameters:
	// - name: userID
	//   in: path
	//   description: User ID
	//   required: true
	//   type: string
	// security:
	// - BearerAuth: []
	// responses:
	//   '200':
	//     description: success
	//   '400':
	//     description: invalid request
	//     schema:
	//       "$ref": "#/definitions/ErrorResponse"
	//   '500':
	//     description: internal error
	//     schema:
	//       "$ref": "#/definitions/ErrorResponse"
	if a.MattermostAuth {
		a.errorResponse(w, r, model.NewErrNotImplemented("not permitted in plugin mode"))
		return
	}

	if len(a.singleUserToken) > 0 {
		// Not permitted in single-user mode
		a.errorResponse(w, r, model.NewErrUnauthorized("not permitted in single-user mode"))
		return
	}

	vars := mux.Vars(r)
	userID := vars["userID"]

	requestBody, err := io.ReadAll(r.Body)
	if err != nil {
		a.errorResponse(w, r, err)
		return
	}

	var requestData model.ChangeUsernameRequest
	if err = json.Unmarshal(requestBody, &requestData); err != nil {
		a.errorResponse(w, r, err)
		return
	}

	if err = requestData.IsValid(); err != nil {
		a.errorResponse(w, r, err)
		return
	}

	auditRec := a.makeAuditRecord(r, "changeUsername", audit.Fail)
	defer a.audit.LogRecord(audit.LevelAuth, auditRec)

	if err = a.app.ChangeUsername(userID, requestData.Password, requestData.NewUsername); err != nil {
		a.errorResponse(w, r, model.NewErrBadRequest(err.Error()))
		return
	}

	jsonStringResponse(w, http.StatusOK, "{}")
	auditRec.Success()
}

func (a *API) sessionRequired(handler func(w http.ResponseWriter, r *http.Request)) func(w http.ResponseWriter, r *http.Request) {
	return a.attachSession(handler, true)
}

func (a *API) attachSession(handler func(w http.ResponseWriter, r *http.Request), required bool) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		token, _ := auth.ParseAuthTokenFromRequest(r)

		a.logger.Debug(`attachSession`, mlog.Bool("single_user", len(a.singleUserToken) > 0))
		if len(a.singleUserToken) > 0 {
			if required && (token != a.singleUserToken) {
				a.errorResponse(w, r, model.NewErrUnauthorized("invalid single user token"))
				return
			}

			now := utils.GetMillis()
			session := &model.Session{
				ID:          model.SingleUser,
				Token:       token,
				UserID:      model.SingleUser,
				AuthService: a.authService,
				Props:       map[string]interface{}{},
				CreateAt:    now,
				UpdateAt:    now,
			}
			ctx := context.WithValue(r.Context(), sessionContextKey, session)
			handler(w, r.WithContext(ctx))
			return
		}

		if a.MattermostAuth && r.Header.Get("Mattermost-User-Id") != "" {
			userID := r.Header.Get("Mattermost-User-Id")
			now := utils.GetMillis()
			session := &model.Session{
				ID:          userID,
				Token:       userID,
				UserID:      userID,
				AuthService: a.authService,
				Props:       map[string]interface{}{},
				CreateAt:    now,
				UpdateAt:    now,
			}

			ctx := context.WithValue(r.Context(), sessionContextKey, session)
			handler(w, r.WithContext(ctx))
			return
		}

		session, err := a.app.GetSession(token)
		if err != nil {
			if required {
				a.errorResponse(w, r, model.NewErrUnauthorized(err.Error()))
				return
			}

			handler(w, r)
			return
		}

		authService := session.AuthService
		if authService != a.authService {
			msg := `Session authService mismatch`
			a.logger.Error(msg,
				mlog.String("sessionID", session.ID),
				mlog.String("want", a.authService),
				mlog.String("got", authService),
			)
			a.errorResponse(w, r, model.NewErrUnauthorized(msg))
			return
		}

		ctx := context.WithValue(r.Context(), sessionContextKey, session)
		handler(w, r.WithContext(ctx))
	}
}

func (a *API) adminRequired(handler func(w http.ResponseWriter, r *http.Request)) func(w http.ResponseWriter, r *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		// Currently, admin APIs require local unix connections
		conn := GetContextConn(r)
		if _, isUnix := conn.(*net.UnixConn); !isUnix {
			a.errorResponse(w, r, model.NewErrUnauthorized("not a local unix connection"))
			return
		}

		handler(w, r)
	}
}
