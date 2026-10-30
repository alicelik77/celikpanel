// SCREEN catalogue part "nginxDefaultHttps": the Nginx page's card about
// visitors to names without a site, CelikPanel's default HTTPS server
// (v0.1.0-alpha.113, prefix nginxdh.). Split from "componentPages", which had
// no headroom left.
// "nginxDefaultHttps" ekran parcasi: Nginx sayfasinda sitesi olmayan adlara
// gelen ziyaretciler karti.

export const enNginxDefaultHttpsScreens = {
    'nginxdh.title': 'Visitors to names without a site',
    'nginxdh.checking': 'Checking how this server answers HTTPS visitors to names without a site…',
    'nginxdh.unknownRead': 'This could not be read from the server, so it is not shown. That does not mean anything is wrong. Nothing was changed. Try again.',
    'nginxdh.label.who': 'Who acts',
    'nginxdh.label.next': 'What to do',
    'nginxdh.label.then': 'What happens next',
    'nginxdh.nobody': 'Nobody. There is nothing for you to do.',
    'nginxdh.active.head': 'Protected',
    'nginxdh.active.reject': 'Someone who opens this server over HTTPS with a name that has no site here (or with its bare IP address) is turned away before any certificate is shown, so they never see the name of one of your sites.',
    'nginxdh.active.placeholder': 'Someone who opens this server over HTTPS with a name that has no site here (or with its bare IP address) gets a placeholder certificate that names no site, and the connection is closed. Their browser shows a warning, but none of your sites’ names is revealed. (Turning them away before any certificate needs nginx 1.19.4 or later; the nginx on this server is older or its version could not be read.)',
    'nginxdh.not_needed.head': 'Not needed yet',
    'nginxdh.not_needed.reason': 'No site on this server uses HTTPS yet, so there is nothing to protect.',
    'nginxdh.not_needed.then': 'CelikPanel sets this up by itself when a site gets its certificate.',
    'nginxdh.pending.head': 'Not set up yet',
    'nginxdh.pending.reason': 'A site here uses HTTPS, but this protection is not in place yet. Until it is, someone who opens this server with a name that has no site here is shown the certificate of one of your sites: their browser warns, and that site’s name is visible.',
    'nginxdh.pending.who': 'You, the server administrator, on this page.',
    'nginxdh.pending.next': 'Press “Set up now”.',
    'nginxdh.pending.then': 'CelikPanel adds one nginx file, has nginx check it, then reloads nginx. If nginx does not accept it, nothing changes and the reason is shown here. CelikPanel also does this by itself the next time a site’s web server settings change.',
    'nginxdh.failed.head': 'Setting this up failed',
    'nginxdh.failed.reason': 'The last attempt did not succeed, so this protection is not in place. nginx keeps running with the settings it had; nothing else was changed. What was reported:',
    'nginxdh.failed.who': 'The server administrator.',
    'nginxdh.failed.next': 'Fix what is reported above (on the server, the command sudo nginx -t shows the same message), then press “Try again”.',
    'nginxdh.failed.then': 'CelikPanel tries once each time you press the button and shows the result here.',
    'nginxdh.owner_default.head': 'Your own setting is used',
    'nginxdh.owner_default.reason': 'The file {file} already makes one of its servers the default for HTTPS, so CelikPanel does not add its own (nginx cannot have two). What a visitor to an unknown name sees is decided by your server.',
    'nginxdh.owner_default.who': 'Nobody, if this is what you want.',
    'nginxdh.owner_default.next': 'To use CelikPanel’s protection instead, remove “default_server” from the port 443 lines of that file and reload nginx, then press “Check again” here.',
    'nginxdh.owner_file.head': 'Your version of the file is kept',
    'nginxdh.owner_file.reason': 'The file {file} was changed or replaced on the server, so CelikPanel leaves it exactly as it is and never writes over it. nginx uses what it contains.',
    'nginxdh.owner_file.who': 'Nobody, if this is what you want.',
    'nginxdh.owner_file.next': 'To go back to CelikPanel’s version, delete that file on the server, press “Check again” here, then “Set up now”.',
    'nginxdh.unknown.head': 'Not known right now',
    'nginxdh.unknown.reason': 'CelikPanel could not read nginx’s settings, so it cannot tell whether this protection is in place. That alone does not mean anything is broken. What was reported:',
    'nginxdh.unknown.who': 'The server administrator.',
    'nginxdh.unknown.next': 'On the server, run sudo nginx -t. If it reports an error, fix the file it names first. Then press “Check again”.',
    'nginxdh.unknown.then': 'Checking only reads; it changes nothing.',
    'nginxdh.setup': 'Set up now',
    'nginxdh.retry': 'Try again',
    'nginxdh.running': 'Setting up…',
    'nginxdh.check': 'Check again',
    'nginxdh.version': 'nginx {version}',
    'nginxdh.versionUnknown': 'nginx version not known',
    'nginxdh.lastAttempt': 'last attempt {time}',
} as const;

export type NginxDefaultHttpsScreenKey = keyof typeof enNginxDefaultHttpsScreens;
