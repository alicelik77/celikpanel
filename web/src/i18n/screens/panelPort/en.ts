// SCREEN catalogue part "panelPort": changing the panel's own port from
// Settings > Panel (PanelPortCard, alpha.114). Key prefix kept here:
// panelPort. (split from "settings").
// One part of the screen copy; ../../index.tsx fetches every part in
// parallel and no screen renders before all of them have arrived.
//
// EKRAN katalogu parçası "panelPort": panelin kendi portunu değiştirmek.

export const enPanelPortScreens = {
    // Panel port (alpha.114): Settings > Panel, PanelPortCard.
    'panelPort.title': 'Panel port',
    'panelPort.checking': 'Reading the panel port…',
    'panelPort.unknown': 'The panel port could not be read, so nothing is shown here. Nothing was changed. Try again.',
    'panelPort.current': 'The panel answers on port {port}.',
    'panelPort.legacy': 'This server was installed before port {port} became the default, so it keeps port {legacy}. Port {legacy} is also the port cPanel uses. You can move the panel to another port here.',
    'panelPort.how': 'The panel will answer on both ports. Open the new address and confirm there. Then the old port closes. If nobody confirms within {minutes} minutes, the panel stays on the old port by itself.',
    'panelPort.input': 'New port (1024–65535)',
    'panelPort.provider': 'If your hosting provider has its own firewall, allow the new port there first. CelikPanel opens it only in this server’s firewall.',
    'panelPort.start': 'Change port',
    'panelPort.label.who': 'Who acts:',
    'panelPort.label.next': 'What to do:',
    'panelPort.label.then': 'What happens next:',
    'panelPort.trial.head': 'The panel now answers on port {to} too',
    'panelPort.trial.who': 'You, in a new browser tab.',
    'panelPort.trial.next': 'Open the new address, sign in if asked, and press “Keep port {to}” there.',
    'panelPort.trial.then': 'Port {from} then closes. If nobody confirms within about {minutes} minutes, the panel stays on port {from} and port {to} closes again.',
    'panelPort.trial.here.head': 'You reached the panel on the new port {to}',
    'panelPort.trial.here.body': 'Keep it, and port {from} closes. Update your bookmarks afterwards.',
    'panelPort.open': 'Open port {to}',
    'panelPort.confirm': 'Keep port {to}',
    'panelPort.cancel': 'Stay on port {from}',
    'panelPort.confirming': 'Saving port {to}. Nothing to do; this takes a few seconds.',
    'panelPort.done.head': 'The panel now uses port {to}',
    'panelPort.done.body': 'Port {from} is closed. Update your bookmarks.',
    'panelPort.back.head': 'The panel stays on port {from}',
    'panelPort.firewallNote': 'Firewall:',
    'panelPort.blocked.head': 'The port cannot be changed now',
    'panelPort.failed': 'The change could not be started. Nothing was changed. Try again.',
    'panelPort.code.PANEL_PORT_RANGE': 'Choose a port from 1024 to 65535. Lower ports belong to system services such as web, mail and DNS.',
    'panelPort.code.PANEL_PORT_RESERVED': 'That port belongs to another service on this server. Choose another port.',
    'panelPort.code.PANEL_PORT_UNCHANGED': 'The panel already uses that port.',
    'panelPort.code.PANEL_PORT_IN_USE': 'Something on this server already listens on that port. Nothing was changed. Choose another port.',
    'panelPort.code.PANEL_PORT_CONFIG_CHANGED': 'The panel’s settings file (/etc/celikpanel/panel.env) names another port than the one the panel runs on. Someone changed it by hand, and CelikPanel does not overwrite it. Restart the panel to use that setting (sudo systemctl restart celikpanel-panel), or change the line back.',
    'panelPort.code.PANEL_PORT_CONFIG_MISSING': 'This panel has no /etc/celikpanel/panel.env, so its port cannot be changed here.',
    'panelPort.code.PANEL_PORT_CONFIG_UNREADABLE': 'The panel’s settings file (/etc/celikpanel/panel.env) cannot be used. The server owner makes it root-only (mode 0600), then tries again.',
    'panelPort.code.PANEL_PORT_AGENT_UNAVAILABLE': 'The CelikPanel agent did not answer, so the settings file could not be read. Nothing was changed. Check that celikpanel-agent is running, then try again.',
    'panelPort.code.PANEL_PORT_PAIRED': 'Another CelikPanel server is linked to this one and reaches it on port {port}. Changing the port would break that link, and this release cannot tell the other server. Keep port {port} on this server.',
    'panelPort.code.PANEL_PORT_CHANGE_OPEN': 'A port change is already waiting. Confirm it from the new address or cancel it first.',
    'panelPort.code.PANEL_PORT_FIREWALL_FAILED': 'The firewall could not open the new port, so the panel stays on its port. Open the Firewall page to see why, then try again.',
    'panelPort.code.PANEL_PORT_CONFIRM_FROM_NEW': 'Confirm from the new address, so the panel knows it really opens for you.',
    'panelPort.code.PANEL_PORT_TIMEOUT': 'Nobody confirmed port {to} in time, so the panel stays on port {from} and port {to} was closed again. If the new address did not open, check your provider’s firewall, then try again.',
    'panelPort.code.PANEL_PORT_CANCELLED': 'The change was cancelled. Port {to} was closed again.',
    'panelPort.code.PANEL_PORT_RESTARTED': 'The panel restarted before port {to} was confirmed, so it stays on port {from}. Start the change again when you are ready.',
    'panelPort.code.PANEL_PORT_CONFIRM_NOT_SAVED': 'The panel stopped while saving port {to}, and it was not saved. The panel stays on port {from}. Start the change again.',
    'panelPort.code.PANEL_PORT_RESTART_NEEDED': 'Port {to} is saved, but the panel still runs on port {from} until it restarts. Restart it (sudo systemctl restart celikpanel-panel), then open the new address.',
    'panelPort.code.PANEL_PORT_SAVE_FAILED': 'The new port could not be saved. The panel still answers on both ports. Try confirming again, or stay on the old port.',
    'panelPort.code.PANEL_PORT_SAVE_UNKNOWN': 'It is not known yet whether the new port was saved. Nothing more is changed. Reload this page in a moment.',
    'panelPort.code.PANEL_PORT_NO_CHANGE': 'No port change is waiting. The page was read again.',
    'panelPort.code.PANEL_PORT_CONFIRMING': 'The new port is being saved now. The page was read again.',
    'panelPort.code.PANEL_PORT_UNAVAILABLE': 'The panel is still starting. Wait a moment, then try again.',
    'panelPort.code.PANEL_PORT_NO_ANSWER': 'No answer arrived. The page was read again and shows what the panel knows now.',
} as const;

export type PanelPortScreenKey = keyof typeof enPanelPortScreens;
