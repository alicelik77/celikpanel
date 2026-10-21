// Batch 22 (2026-10-21): the installed package versions of alpha.97. Answers
// only while `state.b22` is set. It is the batch-20 server (same components,
// bundles and fake install operations) asked through its own state key, so a
// scenario can say which package versions are known:
//
//   server    'boston' (Ubuntu 24.04 sample strings) | 'frankfurt' (Debian 13)
//   versions  'sample' (default) | 'none': the scan carries no package_version* field
//   kept      component ids whose reading is an earlier one (package manager busy)
//   extra     component ids installed on top of the server's own set
//
// Every version string is SAMPLE data, not measured (sample-package-versions.mjs).
// Nothing is installed anywhere; nothing leaves 127.0.0.1.
import { batch20, batch20State } from './mock-batch20.mjs';

export const batch22State = (server, options = {}) => batch20State(server, options);

export async function batch22(req, res, path, query, context) {
    if (!context.state.b22) return false;
    return batch20(req, res, path, query, { ...context, state: { b20: context.state.b22 } });
}
