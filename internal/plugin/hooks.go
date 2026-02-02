package plugin

// Post hooks
const (
	HookPostBeforeCreate = "post.before_create"
	HookPostAfterCreate  = "post.after_create"
	HookPostBeforeUpdate = "post.before_update"
	HookPostAfterUpdate  = "post.after_update"
	HookPostBeforeDelete = "post.before_delete"
	HookPostAfterDelete  = "post.after_delete"
	HookPostContent      = "post.content"
)

// Comment hooks
const (
	HookCommentBeforeCreate = "comment.before_create"
	HookCommentAfterCreate  = "comment.after_create"
	HookCommentBeforeUpdate = "comment.before_update"
	HookCommentAfterUpdate  = "comment.after_update"
	HookCommentBeforeDelete = "comment.before_delete"
	HookCommentAfterDelete  = "comment.after_delete"
	HookCommentContent      = "comment.content"
)

// User hooks
const (
	HookUserAfterLogin = "user.after_login"
)
