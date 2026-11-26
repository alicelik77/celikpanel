package transport

// The Agent's refusal of a privileged database account (alpha.149 security
// fix; review of PR #370, finding 1). The Agent runs as root and as the
// database superuser; the panel is the less trusted side. No Agent database
// call changes an account the panel names unless the Agent itself finds it
// to be an ordinary account (cmd/agent database_account_guard.go). A refusal
// names its reason as one of these codes in the call's AccountRefused field;
// nothing was changed, and the server's administrator decides.
//
// Ayrıcalıklı veritabanı hesabının Agent tarafından reddi: panelin adını
// verdiği hesap, Agent onu sıradan bir hesap olarak bulmadıkça değiştirilmez.
const (
	// DatabaseAccountRefusedAbsent: the account is not on the engine.
	DatabaseAccountRefusedAbsent = "absent"
	// DatabaseAccountRefusedPrivileged: the account holds administrator
	// rights on the engine (directly, through a role, or on its system
	// database).
	DatabaseAccountRefusedPrivileged = "privileged"
	// DatabaseAccountRefusedMember: (PostgreSQL limits only) the role is a
	// member of another role, which no site role CelikPanel makes ever is.
	DatabaseAccountRefusedMember = "member"
	// DatabaseAccountRefusedUnreadable: what the account holds could not be
	// read, so it is not changed.
	DatabaseAccountRefusedUnreadable = "unreadable"
	// DatabaseAccountRefusedSystemDatabase: the call would give an account
	// rights on the engine's own system database (MariaDB mysql,
	// information_schema, performance_schema, sys; PostgreSQL postgres,
	// template0, template1).
	DatabaseAccountRefusedSystemDatabase = "system_database"
	// DatabaseAccountRefusedUnsupportedEngine (second review of PR #370,
	// finding 7): the server runs MySQL in MariaDB's place, so the rule
	// cannot be read; CelikPanel manages database accounts on MariaDB only.
	// Nothing was changed; the server's administrator makes the change on
	// the server itself. An account already gone is answered as absent.
	DatabaseAccountRefusedUnsupportedEngine = "unsupported_engine"
	// DatabaseAccountRefusedOwnsObjects (third review of PR #370, H1): a
	// PostgreSQL user added to a database owned by CelikPanel's own account
	// (celikpanel_admin) owns tables or other objects there, and CelikPanel
	// never hands them to its own account; the user was not removed. The
	// server's administrator hands them to the database's own user on the
	// server, then the user is removed again.
	DatabaseAccountRefusedOwnsObjects = "owns_objects"
)
