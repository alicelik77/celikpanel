// Why a database configuration save was refused, in plain words (8 Nov 2026).
//
// The owner saved an access rule, read "Nothing was saved: a rule you changed
// or added is not one PostgreSQL accepts" at the top of the page and could not
// find why: the reason stood only under the marked rule, far below, and only
// in PostgreSQL's own words. The banner now leads with a plain sentence that
// names the concrete fix, and keeps PostgreSQL's words under it.
//
// The pg_hba.conf lines are the ones the agent's own check gives (Go
// hbaLineError in cmd/agent/db_config_hba.go), which are deterministic, and a
// few that PostgreSQL itself reports. The setting lines are what
// `postgres -C` and `mariadbd --help --verbose` print. A line this table does
// not know gives null: the screen then shows the service's words only, never
// a guess.
//
// Bir veritabanı yapılandırma kaydının neden reddedildiği, düz sözcüklerle.
// Tablonun tanımadığı satır null döner; ekran o zaman yalnız hizmetin kendi
// sözlerini gösterir, tahmin etmez.

export type ConfigReasonKey =
    | 'dbconf.why.hba.type'
    | 'dbconf.why.hba.noDatabase'
    | 'dbconf.why.hba.noUser'
    | 'dbconf.why.hba.noAddress'
    | 'dbconf.why.hba.noMask'
    | 'dbconf.why.hba.badCidr'
    | 'dbconf.why.hba.hostWithMask'
    | 'dbconf.why.hba.badMask'
    | 'dbconf.why.hba.maskFamily'
    | 'dbconf.why.hba.noMethod'
    | 'dbconf.why.hba.badMethod'
    | 'dbconf.why.hba.peerHost'
    | 'dbconf.why.hba.certNoSsl'
    | 'dbconf.why.hba.localGss'
    | 'dbconf.why.hba.multiple'
    | 'dbconf.why.hba.optionFormat'
    | 'dbconf.why.hba.optionName'
    | 'dbconf.why.hba.clientcert'
    | 'dbconf.why.hba.include'
    | 'dbconf.why.hba.sslOff'
    | 'dbconf.why.setting.unknown'
    | 'dbconf.why.setting.invalidValue'
    | 'dbconf.why.setting.needsNumber'
    | 'dbconf.why.setting.needsBoolean'
    | 'dbconf.why.setting.range'
    | 'dbconf.why.setting.syntax'
    | 'dbconf.why.setting.adjusted'
    | 'dbconf.why.setting.notBoolean';

export interface ConfigReason {
    key: ConfigReasonKey;
    /** The value the line names, as it was written; '' when it names none. */
    vars: { value: string };
}

// Each line, the sentence it gets and which group of the match is its value.
// Her satır, aldığı cümle ve eşleşmenin hangi grubunun değeri olduğu.
const table: ReadonlyArray<readonly [RegExp, ConfigReasonKey, number?]> = [
    // pg_hba.conf, as the agent checks it.
    [/^invalid connection type "(.*)"$/, 'dbconf.why.hba.type', 1],
    [/^end-of-line before database specification$/, 'dbconf.why.hba.noDatabase'],
    [/^end-of-line before role specification$/, 'dbconf.why.hba.noUser'],
    [/^end-of-line before IP address specification$/, 'dbconf.why.hba.noAddress'],
    [/^end-of-line before netmask specification$/, 'dbconf.why.hba.noMask'],
    [/^invalid CIDR mask in address "(.*)"$/, 'dbconf.why.hba.badCidr', 1],
    [/^specifying both host name and CIDR mask is invalid: "(.*)"$/, 'dbconf.why.hba.hostWithMask', 1],
    [/^invalid IP mask "(.*)"/, 'dbconf.why.hba.badMask', 1],
    [/^IP address and mask do not match/, 'dbconf.why.hba.maskFamily'],
    [/^end-of-line before authentication method$/, 'dbconf.why.hba.noMethod'],
    [/^invalid authentication method "(.*)"$/, 'dbconf.why.hba.badMethod', 1],
    [/^peer authentication is only supported on local sockets$/, 'dbconf.why.hba.peerHost'],
    [/^cert authentication is only supported on hostssl connections$/, 'dbconf.why.hba.certNoSsl'],
    [/^(gss|sspi) authentication is not supported on local sockets$/, 'dbconf.why.hba.localGss', 1],
    [/^multiple values specified for /, 'dbconf.why.hba.multiple'],
    [/^authentication option not in name=value format: (.*)$/, 'dbconf.why.hba.optionFormat', 1],
    [/^unrecognized authentication option name: "(.*)"$/, 'dbconf.why.hba.optionName', 1],
    [/^clientcert can only be configured for "hostssl" rows$/, 'dbconf.why.hba.clientcert'],
    [/^an include directive takes exactly one file or directory name$/, 'dbconf.why.hba.include'],
    // pg_hba.conf, as PostgreSQL itself reports it after the agent's check.
    [/^hostssl record cannot match because SSL is disabled/, 'dbconf.why.hba.sslOff'],
    // postgresql.conf, as `postgres -C` prints it.
    [/^unrecognized configuration parameter "([^"]*)"/, 'dbconf.why.setting.unknown', 1],
    [/^parameter "[^"]*" requires a Boolean value/, 'dbconf.why.setting.needsBoolean'],
    [/^parameter "[^"]*" requires a numeric value/, 'dbconf.why.setting.needsNumber'],
    [/^(\S+) is outside the valid range for parameter "[^"]*" \(([^)]*)\)/, 'dbconf.why.setting.range', 2],
    [/^invalid value for parameter "[^"]*": "(.*)"/, 'dbconf.why.setting.invalidValue', 1],
    [/^syntax error in file /, 'dbconf.why.setting.syntax'],
    // A MariaDB option file, as mariadbd prints it.
    [/unknown (?:variable|option) '(?:--)?([A-Za-z0-9_.-]+)/, 'dbconf.why.setting.unknown', 1],
    [/^option '[^']*': .*\badjusted to (\S+?)\.?$/, 'dbconf.why.setting.adjusted', 1],
    [/^option '[^']*': boolean value '[^']*' wasn't recognized\. Set to (\w+)/, 'dbconf.why.setting.notBoolean', 1],
];

/**
 * The plain sentence for one line a configuration check gave, or null when
 * this table does not know the line.
 */
export function plainConfigReason(detail: string): ConfigReason | null {
    const line = detail.trim();
    if (!line) return null;
    for (const [pattern, key, group] of table) {
        const match = pattern.exec(line);
        if (match) return { key, vars: { value: group ? match[group] ?? '' : '' } };
    }
    return null;
}
