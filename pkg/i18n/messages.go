package i18n

// DefaultMessages returns built-in translations for all supported locales.
// These can be overridden by loading JSON files from a directory.
func DefaultMessages() map[Locale]map[string]string {
	return map[Locale]map[string]string{
		LocaleKo: koMessages,
		LocaleEn: enMessages,
		LocaleJa: jaMessages,
	}
}

var koMessages = map[string]string{
	// Common errors
	"error.not_found":         "요청한 리소스를 찾을 수 없습니다",
	"error.unauthorized":      "인증이 필요합니다",
	"error.forbidden":         "접근 권한이 없습니다",
	"error.bad_request":       "잘못된 요청입니다",
	"error.internal":          "서버 내부 오류가 발생했습니다",
	"error.too_many_requests": "요청이 너무 많습니다. 잠시 후 다시 시도해주세요",
	"error.validation":        "입력값이 올바르지 않습니다",

	// Auth
	"auth.login_success":      "로그인 되었습니다",
	"auth.login_failed":       "아이디 또는 비밀번호가 올바르지 않습니다",
	"auth.token_expired":      "인증 토큰이 만료되었습니다. 다시 로그인해주세요",
	"auth.token_invalid":      "유효하지 않은 인증 토큰입니다",
	"auth.register_success":   "회원가입이 완료되었습니다",
	"auth.duplicate_id":       "이미 사용 중인 아이디입니다",
	"auth.duplicate_email":    "이미 사용 중인 이메일입니다",
	"auth.duplicate_nickname": "이미 사용 중인 닉네임입니다",
	"auth.withdraw_success":   "회원 탈퇴가 완료되었습니다",
	"auth.logout_success":     "로그아웃 되었습니다",

	// Posts
	"post.not_found":      "게시글을 찾을 수 없습니다",
	"post.create_success": "게시글이 작성되었습니다",
	"post.update_success": "게시글이 수정되었습니다",
	"post.delete_success": "게시글이 삭제되었습니다",
	"post.not_owner":      "본인이 작성한 게시글만 수정/삭제할 수 있습니다",

	// Comments
	"comment.not_found":      "댓글을 찾을 수 없습니다",
	"comment.create_success": "댓글이 작성되었습니다",
	"comment.delete_success": "댓글이 삭제되었습니다",

	// Members
	"member.not_found": "회원을 찾을 수 없습니다",
	"member.blocked":   "차단된 회원입니다",
	"member.suspended": "이용이 정지된 계정입니다",

	// Files
	"file.too_large":        "파일 크기가 제한을 초과했습니다",
	"file.type_not_allowed": "허용되지 않는 파일 형식입니다",
	"file.upload_success":   "파일이 업로드되었습니다",
	"file.delete_success":   "파일이 삭제되었습니다",

	// Rate limit
	"rate_limit.exceeded": "요청 제한을 초과했습니다. %d초 후 다시 시도해주세요",

	// CSRF
	"csrf.missing": "CSRF 토큰이 누락되었습니다",
	"csrf.invalid": "CSRF 토큰이 유효하지 않습니다",

	// Search
	"search.query_required": "검색어를 입력해주세요",
}

var enMessages = map[string]string{
	// Common errors
	"error.not_found":         "The requested resource was not found",
	"error.unauthorized":      "Authentication is required",
	"error.forbidden":         "You do not have permission to access this resource",
	"error.bad_request":       "Invalid request",
	"error.internal":          "An internal server error occurred",
	"error.too_many_requests": "Too many requests. Please try again later",
	"error.validation":        "Invalid input",

	// Auth
	"auth.login_success":      "Successfully logged in",
	"auth.login_failed":       "Invalid username or password",
	"auth.token_expired":      "Authentication token has expired. Please login again",
	"auth.token_invalid":      "Invalid authentication token",
	"auth.register_success":   "Registration completed",
	"auth.duplicate_id":       "This user ID is already taken",
	"auth.duplicate_email":    "This email is already registered",
	"auth.duplicate_nickname": "This nickname is already taken",
	"auth.withdraw_success":   "Account has been deleted",
	"auth.logout_success":     "Successfully logged out",

	// Posts
	"post.not_found":      "Post not found",
	"post.create_success": "Post created successfully",
	"post.update_success": "Post updated successfully",
	"post.delete_success": "Post deleted successfully",
	"post.not_owner":      "You can only edit/delete your own posts",

	// Comments
	"comment.not_found":      "Comment not found",
	"comment.create_success": "Comment posted successfully",
	"comment.delete_success": "Comment deleted successfully",

	// Members
	"member.not_found": "Member not found",
	"member.blocked":   "This member has been blocked",
	"member.suspended": "This account has been suspended",

	// Files
	"file.too_large":        "File size exceeds the limit",
	"file.type_not_allowed": "File type is not allowed",
	"file.upload_success":   "File uploaded successfully",
	"file.delete_success":   "File deleted successfully",

	// Rate limit
	"rate_limit.exceeded": "Rate limit exceeded. Please retry after %d seconds",

	// CSRF
	"csrf.missing": "CSRF token is missing",
	"csrf.invalid": "Invalid CSRF token",

	// Search
	"search.query_required": "Search query is required",
}

var jaMessages = map[string]string{
	// Common errors
	"error.not_found":         "リクエストされたリソースが見つかりません",
	"error.unauthorized":      "認証が必要です",
	"error.forbidden":         "アクセス権限がありません",
	"error.bad_request":       "無効なリクエストです",
	"error.internal":          "サーバー内部エラーが発生しました",
	"error.too_many_requests": "リクエストが多すぎます。しばらくしてから再試行してください",
	"error.validation":        "入力値が正しくありません",

	// Auth
	"auth.login_success":      "ログインしました",
	"auth.login_failed":       "IDまたはパスワードが正しくありません",
	"auth.token_expired":      "認証トークンの有効期限が切れました。再度ログインしてください",
	"auth.token_invalid":      "無効な認証トークンです",
	"auth.register_success":   "会員登録が完了しました",
	"auth.duplicate_id":       "このIDは既に使用されています",
	"auth.duplicate_email":    "このメールアドレスは既に登録されています",
	"auth.duplicate_nickname": "このニックネームは既に使用されています",
	"auth.withdraw_success":   "退会が完了しました",
	"auth.logout_success":     "ログアウトしました",

	// Posts
	"post.not_found":      "投稿が見つかりません",
	"post.create_success": "投稿が作成されました",
	"post.update_success": "投稿が更新されました",
	"post.delete_success": "投稿が削除されました",
	"post.not_owner":      "自分の投稿のみ編集・削除できます",

	// Comments
	"comment.not_found":      "コメントが見つかりません",
	"comment.create_success": "コメントが投稿されました",
	"comment.delete_success": "コメントが削除されました",

	// Members
	"member.not_found": "会員が見つかりません",
	"member.blocked":   "ブロックされた会員です",
	"member.suspended": "利用停止されたアカウントです",

	// Files
	"file.too_large":        "ファイルサイズが制限を超えています",
	"file.type_not_allowed": "許可されていないファイル形式です",
	"file.upload_success":   "ファイルがアップロードされました",
	"file.delete_success":   "ファイルが削除されました",

	// Rate limit
	"rate_limit.exceeded": "リクエスト制限を超えました。%d秒後に再試行してください",

	// CSRF
	"csrf.missing": "CSRFトークンが不足しています",
	"csrf.invalid": "無効なCSRFトークンです",

	// Search
	"search.query_required": "検索語を入力してください",
}
