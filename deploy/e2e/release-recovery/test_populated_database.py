#!/usr/bin/env python3
"""Component tests only; these do not establish native migration interruption."""
from __future__ import annotations

import hashlib
import importlib.util
import json
import os
from pathlib import Path
import signal
import sqlite3
import sys
import tempfile
import unittest
from unittest import mock

HERE = Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location('populated_database_under_test', HERE / 'populated_database.py')
p = importlib.util.module_from_spec(spec)
spec.loader.exec_module(p)
MIGRATION_DIR = HERE.parents[2] / 'internal' / 'db' / 'migrations'
LEDGER_SQL = '''
\tCREATE TABLE IF NOT EXISTS schema_migrations (
\t\tversion INTEGER PRIMARY KEY,
\t\tfilename TEXT,
\t\tsha256 TEXT,
\t\tapplied_at TEXT DEFAULT (datetime('now'))
\t)'''


def encoded(value):
    return json.dumps(value, sort_keys=True, ensure_ascii=True, separators=(',', ':')).encode()


def digest(value):
    return hashlib.sha256(value).hexdigest()


def migrations():
    # Clean archive and Windows checkout both work; exact LF source identity is
    # pinned against the real Alpha64 commit, not fabricated test migrations.
    result = []
    for path in sorted(MIGRATION_DIR.glob('*.sql')):
        raw = path.read_bytes().replace(b'\r\n', b'\n')
        result.append((int(path.name[:3]), path.name, digest(raw), raw.decode()))
    return result


def apply(connection, records):
    for version, filename, sha256, sql in records:
        connection.executescript('BEGIN;\n' + sql)
        connection.execute('INSERT INTO schema_migrations(version,filename,sha256,applied_at) VALUES(?,?,?,?)',
                           (version, filename, sha256, '2000-01-01 00:00:00'))
        connection.commit()


@unittest.skipUnless(sys.platform == 'linux', 'private Linux filesystem contract')
class PopulatedDatabaseTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.sql = migrations()
        old_identities = [{'version': v, 'filename': n, 'sha256': h} for v, n, h, _ in cls.sql[:38]]
        if len(old_identities) != 38 or digest(encoded(old_identities)) != p.MIGRATIONS[38]:
            raise AssertionError('test input must be the exact released Alpha64 migration bytes')
        cls.seed_root = tempfile.TemporaryDirectory(prefix='cp-populated-tests-')
        base = Path(cls.seed_root.name) / 'base.db'
        connection = sqlite3.connect(base)
        connection.execute('PRAGMA foreign_keys=ON')
        connection.execute(LEDGER_SQL)
        apply(connection, cls.sql[:38])
        # Prior nonempty owner rows make preservation non-vacuous. This remains
        # a SQL fixture; it does not pretend to create any native service.
        connection.execute("INSERT INTO users(id,username,password_hash,email,role,created_at) VALUES(81,'prior-owner','PRIVATE-ADMIN-VALUE','prior@example.test','customer','1999-01-01')")
        connection.execute("INSERT INTO subscriptions(id,owner_id,name) VALUES(83,81,'Existing subscription')")
        connection.execute("INSERT INTO domains(id,subscription_id,name,status) VALUES(85,83,'prior.example.test','active')")
        connection.execute("INSERT INTO domain_aliases(id,domain_id,alias) VALUES(87,85,'prior-alias.example.test')")
        connection.execute("INSERT INTO metrics_samples VALUES('2000-01-01T00:00:00Z',1.25,123,999,456,888,0.75)")
        connection.commit()
        p._schema(connection, 38)
        connection.close()
        cls.baseline_bytes = base.read_bytes()

    @classmethod
    def tearDownClass(cls):
        cls.seed_root.cleanup()

    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix='cp-populated-case-')
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.source = self.root / 'original.db'
        self.source.write_bytes(self.baseline_bytes)
        self.source.chmod(0o600)
        self.output = self.root / 'outputs'
        self.output.mkdir(mode=0o700)
        self.admission = {'schema': p.ADMISSION_SCHEMA, 'purpose': p.PURPOSE,
                          'baseline_commit': p.BASELINE_COMMIT, 'baseline_migrations_sha256': p.MIGRATIONS[38],
                          'source_sha256': digest(self.baseline_bytes), 'nonce': 'a1' * 16}

    def build(self):
        return p.build_private_copy(self.source, self.output, self.admission)

    def verify(self, result, version=38, path=None):
        return p.verify_copy(Path(path or result['database']), result['manifest'],
                             manifest_sha256=result['manifest_sha256'], expected_version=version)

    def change_source(self, sql, parameters=()):
        connection = sqlite3.connect(self.source)
        connection.execute(sql, parameters)
        connection.commit()
        connection.close()
        self.admission['source_sha256'] = digest(self.source.read_bytes())

    def test_all_old_rows_relations_private_output_and_no_secret_values(self):
        before = self.source.stat()
        result = self.build()
        proof = self.verify(result)
        self.assertEqual(proof['old_table_count'], 55)
        self.assertEqual(proof['table_count'], 55)
        self.assertEqual(proof['fixture_domain_count'], 2)
        self.assertEqual(proof['fixture_hostname_reservations'], 6)
        self.assertEqual(proof['old_rows_missing_or_changed'], 0)
        self.assertEqual(proof['excluded_tables'], [])
        self.assertTrue(proof['global_bytes_equal_to_populated'])
        self.assertEqual(self.source.read_bytes(), self.baseline_bytes)
        self.assertEqual(p._identity(self.source.stat()), p._identity(before))
        directory = Path(result['directory'])
        self.assertEqual(directory.stat().st_mode & 0o777, 0o700)
        self.assertEqual({f.name for f in directory.iterdir()}, {'admission.json', 'celikpanel.db', 'manifest.json'})
        self.assertTrue(all((f.stat().st_mode & 0o777) == 0o600 for f in directory.iterdir()))
        manifest = (directory / 'manifest.json').read_bytes()
        self.assertNotIn(b'PRIVATE-ADMIN-VALUE', manifest)
        self.assertNotIn(b'prior@example.test', manifest)
        self.assertNotIn(b'Existing subscription', manifest)
        self.assertEqual(digest(manifest), result['manifest_sha256'])
        self.assertEqual(len(result['manifest']['tables']['domains']['rows']), 3)

    def test_actual_39_through_42_migrations_preserve_all_old_columns_and_defaults(self):
        result = self.build()
        connection = sqlite3.connect(result['database'])
        connection.execute('PRAGMA foreign_keys=ON')
        apply(connection, self.sql[38:42])
        connection.close()
        proof = self.verify(result, 42)
        self.assertEqual(proof['schema_version'], 42)
        self.assertEqual(proof['table_count'], 65)
        self.assertTrue(proof['domain_defaults_40_42_verified'])
        self.assertFalse(proof['global_bytes_equal_to_populated'])
        self.assertEqual(proof['tables']['schema_migrations']['added'], 4)
        self.assertEqual(proof['tables']['domains']['before'], 3)

    def test_actual_39_through_43_migrations_add_one_empty_table_and_change_no_old_row(self):
        # Migration 43 (request identities, D-029). A candidate that carries it
        # is held to its own pinned digests; schema42 evidence does not cover it.
        self.assertGreaterEqual(len(self.sql), 43)
        result = self.build()
        connection = sqlite3.connect(result['database'])
        connection.execute('PRAGMA foreign_keys=ON')
        apply(connection, self.sql[38:43])
        connection.close()
        with self.assertRaises(p.Refused):
            self.verify(result, 42)
        proof = self.verify(result, 43)
        self.assertEqual(proof['schema_version'], 43)
        self.assertEqual(proof['table_count'], 66)
        self.assertEqual(proof['old_table_count'], 55)
        self.assertTrue(proof['domain_defaults_40_42_verified'])
        self.assertEqual(proof['old_rows_missing_or_changed'], 0)
        self.assertEqual(proof['tables']['schema_migrations']['added'], 5)
        self.assertEqual(p.NEW_TABLES[43] - p.NEW_TABLES[42], {'request_identities'})
        for version in (45, 41, '43', True):
            with self.subTest(version=version), self.assertRaises(p.Refused):
                self.verify(result, version)

    def test_actual_39_through_44_migrations_add_two_empty_tables_and_change_no_old_row(self):
        # Migration 44 (managed site files, D-031). The table starts empty: the
        # migration writes no row and touches no file; rows come from the first
        # start that classifies the vhosts.
        result = self.build()
        connection = sqlite3.connect(result['database'])
        connection.execute('PRAGMA foreign_keys=ON')
        apply(connection, self.sql[38:44])
        connection.close()
        for wrong in (42, 43):
            with self.assertRaises(p.Refused):
                self.verify(result, wrong)
        proof = self.verify(result, 44)
        self.assertEqual(proof['schema_version'], 44)
        self.assertEqual(proof['table_count'], 67)
        self.assertEqual(proof['old_table_count'], 55)
        self.assertEqual(proof['old_rows_missing_or_changed'], 0)
        self.assertEqual(proof['tables']['schema_migrations']['added'], 6)
        self.assertEqual(p.NEW_TABLES[44] - p.NEW_TABLES[43], {'managed_site_files'})
        for version in (45, '44', True):
            with self.subTest(version=version), self.assertRaises(p.Refused):
                self.verify(result, version)

    def test_actual_39_through_45_migrations_add_two_null_columns_and_change_no_old_row(self):
        # Migration 45 (alpha.107, D-038): audit_logs.details and
        # sites.app_run_as, NULL in every old row; no table is added.
        result = self.build()
        connection = sqlite3.connect(result['database'])
        connection.execute('PRAGMA foreign_keys=ON')
        apply(connection, self.sql[38:45])
        connection.close()
        for wrong in (43, 44):
            with self.assertRaises(p.Refused):
                self.verify(result, wrong)
        proof = self.verify(result, 45)
        self.assertEqual(proof['schema_version'], 45)
        self.assertEqual(proof['table_count'], 67)
        self.assertEqual(proof['old_rows_missing_or_changed'], 0)
        self.assertEqual(proof['tables']['schema_migrations']['added'], 7)
        self.assertEqual(p.NEW_TABLES[45], p.NEW_TABLES[44])
        self.assertEqual(p.added_columns(45), {'audit_logs': ('details',), 'sites': ('app_run_as',)})
        self.assertEqual(p.added_columns(44), {})
        for version in (47, '45', True):
            with self.subTest(version=version), self.assertRaises(p.Refused):
                self.verify(result, version)

    def test_actual_39_through_46_migrations_add_a_null_column_and_a_table_and_change_no_old_row(self):
        # Migration 46 (alpha.108, PostgreSQL majors side by side):
        # database_servers.cluster, NULL in every old row, and the empty
        # postgresql_database_moves table.
        result = self.build()
        connection = sqlite3.connect(result['database'])
        connection.execute('PRAGMA foreign_keys=ON')
        apply(connection, self.sql[38:46])
        connection.close()
        for wrong in (44, 45):
            with self.assertRaises(p.Refused):
                self.verify(result, wrong)
        proof = self.verify(result, 46)
        self.assertEqual(proof['schema_version'], 46)
        self.assertEqual(proof['table_count'], 68)
        self.assertEqual(proof['old_rows_missing_or_changed'], 0)
        self.assertEqual(proof['tables']['schema_migrations']['added'], 8)
        self.assertEqual(p.NEW_TABLES[46] - p.NEW_TABLES[45], {'postgresql_database_moves'})
        self.assertEqual(p.added_columns(46), {'audit_logs': ('details',), 'sites': ('app_run_as',), 'database_servers': ('cluster',)})
        for version in (47, '46', True):
            with self.subTest(version=version), self.assertRaises(p.Refused):
                self.verify(result, version)

    def test_actual_39_through_51_migrations_add_tables_and_null_columns_and_change_no_old_row(self):
        # alpha.114 (47-51), all additive: eight new tables; NULL
        # sessions.impersonator_id, audit_logs.acting_as and
        # users.email_verified_at in every old row.
        result = self.build()
        connection = sqlite3.connect(result['database'])
        connection.execute('PRAGMA foreign_keys=ON')
        apply(connection, self.sql[38:51])
        connection.close()
        for wrong in (49, 50):
            with self.assertRaises(p.Refused):
                self.verify(result, wrong)
        proof = self.verify(result, 51)
        self.assertEqual(proof['schema_version'], 51)
        self.assertEqual(proof['table_count'], 76)
        self.assertEqual(proof['old_rows_missing_or_changed'], 0)
        self.assertEqual(proof['tables']['schema_migrations']['added'], 13)
        self.assertEqual(p.NEW_TABLES[51] - p.NEW_TABLES[46], {
            'panel_port_changes', 'reseller_pools', 'account_links', 'site_protected_dirs',
            'site_protected_users', 'site_redirects', 'two_factor_recovery_codes', 'email_autoresponders'})
        self.assertEqual(p.added_columns(51), {
            'audit_logs': ('details', 'acting_as'), 'sites': ('app_run_as',), 'database_servers': ('cluster',),
            'sessions': ('impersonator_id',), 'users': ('email_verified_at',)})
        for version in (52, '51', True):
            with self.subTest(version=version), self.assertRaises(p.Refused):
                self.verify(result, version)

    def test_actual_39_through_54_migrations_add_tables_and_null_columns_and_change_no_old_row(self):
        # alpha.117 (52-54): eleven new tables; NULL subscriptions.starts_at,
        # ends_at, suspend_reason and suspended_at in every old row. 52 also
        # writes rights rows: each plan-less customer subscription becomes
        # Custom; every customer here already has a subscription, so no
        # 'Unlimited' one and no notice is added (see the next test).
        result = self.build()
        connection = sqlite3.connect(result['database'])
        connection.execute('PRAGMA foreign_keys=ON')
        apply(connection, self.sql[38:54])
        connection.close()
        for wrong in (51, 52, 53):
            with self.subTest(wrong=wrong), self.assertRaises(p.Refused):
                self.verify(result, wrong)
        proof = self.verify(result, 54)
        self.assertEqual(proof['schema_version'], 54)
        self.assertEqual(proof['table_count'], 87)
        self.assertEqual(proof['old_rows_missing_or_changed'], 0)
        self.assertEqual(proof['tables']['schema_migrations']['added'], 16)
        self.assertEqual(proof['tables']['subscriptions']['added'], 0)
        self.assertEqual(proof['unlimited_subscriptions_52'], 0)
        self.assertEqual(p.NEW_TABLES[54] - p.NEW_TABLES[51], {
            'rights_sets', 'rights_set_items', 'reseller_plans', 'reseller_plan_bindings', 'rights_notices',
            'subscription_suspensions', 'reseller_suspensions', 'suspension_operations', 'suspension_effects',
            'suspension_before_images', 'subscription_transfers'})
        self.assertEqual(p.added_columns(54), {
            'audit_logs': ('details', 'acting_as'), 'sites': ('app_run_as',), 'database_servers': ('cluster',),
            'sessions': ('impersonator_id',), 'users': ('email_verified_at',),
            'subscriptions': ('starts_at', 'ends_at', 'suspend_reason', 'suspended_at')})
        connection = sqlite3.connect(Path(result['database']).as_uri() + '?mode=ro&immutable=1', uri=True)
        subscriptions = [row[0] for row in connection.execute('SELECT id FROM subscriptions WHERE plan_id IS NULL ORDER BY id')]
        self.assertEqual(connection.execute(
            "SELECT owner_id,all_permissions,all_software FROM rights_sets WHERE owner_kind='subscription' ORDER BY owner_id").fetchall(),
            [(i, 0, 1) for i in subscriptions])
        self.assertEqual(connection.execute('SELECT count(*) FROM rights_notices').fetchone(), (0,))
        connection.close()
        for version in (55, '54', True):
            with self.subTest(version=version), self.assertRaises(p.Refused):
                self.verify(result, version)

    def migrated_54_with_unlimited_customer(self, tamper=None):
        # Old data 52 acts on: customer 91 without a subscription, its team
        # member 92 (gets nothing), and subscription 83 with the unused
        # 001 expires_at set to a past date.
        self.change_source("INSERT INTO users(id,username,password_hash,email,role,created_at) "
                           "VALUES(91,'no-subscription','!','none@example.test','customer','1999-01-01')")
        self.change_source("INSERT INTO users(id,username,password_hash,email,role,parent_id,account_type,created_at) "
                           "VALUES(92,'member','!','member@example.test','customer',91,'additional_user','1999-01-01')")
        self.change_source("UPDATE subscriptions SET expires_at='2001-01-01 00:00:00' WHERE id=83")
        result = self.build()
        connection = sqlite3.connect(result['database'])
        connection.execute('PRAGMA foreign_keys=ON')
        apply(connection, self.sql[38:54])
        if tamper:
            connection.execute(tamper)
            connection.commit()
        connection.close()
        return result

    def test_migration52_unlimited_subscription_and_unused_expiry_are_verified_exactly(self):
        # K4: the customer without a subscription gets one 'Unlimited'
        # subscription and a notice; the subscriptions counter advances to
        # it. expires_at is not copied, so ends_at stays NULL (no suspension
        # at the first start after the update).
        result = self.migrated_54_with_unlimited_customer()
        proof = self.verify(result, 54)
        self.assertEqual(proof['old_rows_missing_or_changed'], 0)
        self.assertEqual(proof['unlimited_subscriptions_52'], 1)
        self.assertEqual(proof['tables']['subscriptions']['added'], 1)
        connection = sqlite3.connect(Path(result['database']).as_uri() + '?mode=ro&immutable=1', uri=True)
        self.assertEqual(connection.execute('SELECT expires_at,ends_at FROM subscriptions WHERE id=83').fetchone(),
                         ('2001-01-01 00:00:00', None))
        self.assertEqual(connection.execute("SELECT owner_id,name FROM subscriptions WHERE owner_id IN (91,92)").fetchall(),
                         [(91, 'Unlimited')])
        connection.close()

    def test_migration52_transform_is_the_only_accepted_change(self):
        for tamper in ("UPDATE subscriptions SET max_domains=1 WHERE name='Unlimited'",
                       "INSERT INTO subscriptions(owner_id,name) VALUES(81,'second')",
                       "INSERT INTO subscriptions(owner_id,name,status,starts_at) VALUES(81,'Unlimited','active','2000-01-01 00:00:00')",
                       "DELETE FROM rights_notices WHERE kind='unlimited_after_migration'",
                       "UPDATE sqlite_sequence SET seq=seq+5 WHERE name='subscriptions'",
                       "UPDATE subscriptions SET ends_at=expires_at WHERE id=83"):
            with self.subTest(tamper=tamper):
                self.setUp()
                result = self.migrated_54_with_unlimited_customer(tamper)
                with self.assertRaises(p.Refused):
                    self.verify(result, 54)

    def test_actual_39_through_55_migrations_add_tables_and_change_no_old_row(self):
        # alpha.117 (55, D-043): six new tables; no old table gains a column.
        result = self.build()
        connection = sqlite3.connect(result['database'])
        connection.execute('PRAGMA foreign_keys=ON')
        apply(connection, self.sql[38:55])
        connection.close()
        for wrong in (53, 54):
            with self.subTest(wrong=wrong), self.assertRaises(p.Refused):
                self.verify(result, wrong)
        proof = self.verify(result, 55)
        self.assertEqual(proof['schema_version'], 55)
        self.assertEqual(proof['table_count'], 93)
        self.assertEqual(proof['old_rows_missing_or_changed'], 0)
        self.assertEqual(proof['tables']['schema_migrations']['added'], 17)
        self.assertEqual(p.NEW_TABLES[55] - p.NEW_TABLES[54], {
            'security_emergencies', 'security_emergency_operations', 'security_emergency_effects',
            'security_emergency_sites', 'security_emergency_before_images', 'security_notices'})
        self.assertEqual(p.added_columns(55), p.added_columns(54))
        for version in (56, '55', True):
            with self.subTest(version=version), self.assertRaises(p.Refused):
                self.verify(result, version)

    def test_actual_39_through_56_migrations_add_tables_and_change_no_old_row(self):
        # alpha.117 (56): two transfer rotation tables; no old table gains a column.
        result = self.build()
        connection = sqlite3.connect(result['database'])
        connection.execute('PRAGMA foreign_keys=ON')
        apply(connection, self.sql[38:56])
        connection.close()
        for wrong in (54, 55):
            with self.subTest(wrong=wrong), self.assertRaises(p.Refused):
                self.verify(result, wrong)
        proof = self.verify(result, 56)
        self.assertEqual(proof['schema_version'], 56)
        self.assertEqual(proof['table_count'], 95)
        self.assertEqual(proof['old_rows_missing_or_changed'], 0)
        self.assertEqual(proof['tables']['schema_migrations']['added'], 18)
        self.assertEqual(p.NEW_TABLES[56] - p.NEW_TABLES[55], {'transfer_rotations', 'transfer_rotation_effects'})
        self.assertEqual(p.added_columns(56), p.added_columns(55))
        for version in (57, '56', True):
            with self.subTest(version=version), self.assertRaises(p.Refused):
                self.verify(result, version)

    def migrated_to(self, version):
        result = self.build()
        connection = sqlite3.connect(result['database'])
        connection.execute('PRAGMA foreign_keys=ON')
        apply(connection, self.sql[38:version])
        connection.close()
        return result

    def test_actual_39_through_57_migrations_reset_only_the_traffic_limit_of_old_rows(self):
        # alpha.118 (57, D-044): six new tables; every old subscriptions and
        # service_plans bandwidth_quota_mb becomes 0 (unlimited). Old rows
        # held the schema38 default 102400, so the reset is proved row by
        # row and nothing else in them may differ.
        result = self.migrated_to(57)
        for wrong in (55, 56):
            with self.subTest(wrong=wrong), self.assertRaises(p.Refused):
                self.verify(result, wrong)
        proof = self.verify(result, 57)
        self.assertEqual(proof['schema_version'], 57)
        self.assertEqual(proof['table_count'], 101)
        self.assertEqual(proof['old_rows_missing_or_changed'], 0)
        self.assertEqual(proof['tables']['schema_migrations']['added'], 19)
        self.assertGreaterEqual(proof['traffic_limits_reset_57'], 2)
        self.assertEqual(p.NEW_TABLES[57] - p.NEW_TABLES[56], {
            'traffic_daily', 'traffic_collect_state', 'traffic_sources', 'traffic_web_blocks',
            'subscription_disk_usage', 'usage_notices'})
        self.assertEqual(p.added_columns(57), p.added_columns(56))
        for version in (58, '57', True):
            with self.subTest(version=version), self.assertRaises(p.Refused):
                self.verify(result, version)

    def test_migration57_reset_accepts_only_known_old_traffic_values_and_no_other_change(self):
        # An old limit other than NULL or 102400 cannot be reproduced from the
        # sealed digest and stays refused; so does any other changed column.
        self.change_source("UPDATE subscriptions SET bandwidth_quota_mb=5000 WHERE id=83")
        result = self.migrated_to(57)
        with self.assertRaises(p.Refused):
            self.verify(result, 57)
        self.setUp()
        result = self.migrated_to(57)
        connection = sqlite3.connect(result['database'])
        connection.execute("UPDATE subscriptions SET name='changed' WHERE id=83")
        connection.commit()
        connection.close()
        with self.assertRaises(p.Refused):
            self.verify(result, 57)

    def test_actual_39_through_72_migrations_add_tables_and_change_no_other_old_row(self):
        # alpha.118: 58 application catalog operations, 59 WordPress tools and
        # malware scans, 60 remote backups, 61 service apps; all additive.
        # alpha.119: 62 the first-login tour's state (one table, one trigger),
        # 63 runtime apps (three tables).
        # alpha.126: 64 monitoring history (one table, one NULL column on the
        # old metrics_samples, which every old row keeps NULL).
        # alpha.127: 65 monitoring alerts (three tables, four NULL columns on
        # the old metrics_samples).
        # alpha.128: 66 site setting limits (two tables; NOT NULL defaults on
        # the old service_plans and subscriptions, proved equal to the default
        # in every old row), 67 outgoing mail, 68 web server settings, 69
        # visitor statistics, 70 remote API tokens, 71 additional FTP/SFTP
        # logins, 72 panel operations (three NULL columns on the old
        # sessions); all additive.
        self.assertEqual(len(self.sql), 72, 'a migration after 72 needs its own pin before a candidate carries it')
        counts = {58: 102, 59: 107, 60: 110, 61: 113, 62: 114, 63: 117, 64: 118, 65: 121,
                  66: 123, 67: 129, 68: 134, 69: 135, 70: 136, 71: 137, 72: 140}
        for version, tables in counts.items():
            with self.subTest(version=version):
                self.setUp()
                result = self.migrated_to(version)
                with self.assertRaises(p.Refused):
                    self.verify(result, version - 1)
                proof = self.verify(result, version)
                self.assertEqual(proof['table_count'], tables)
                self.assertEqual(proof['old_rows_missing_or_changed'], 0)
                self.assertEqual(proof['tables']['schema_migrations']['added'], version - 38)
                expected_columns = dict(p.added_columns(56))
                if version >= 64:
                    expected_columns['metrics_samples'] = ('cpu_peak',)
                if version >= 65:
                    expected_columns['metrics_samples'] += ('net_rx', 'net_tx', 'disk_read', 'disk_write')
                if version >= 72:
                    expected_columns['sessions'] = expected_columns.get('sessions', ()) + ('last_seen_at', 'client_ip', 'user_agent')
                self.assertEqual(p.added_columns(version), expected_columns)
        for version in (73, '72', True):
            with self.subTest(version=version), self.assertRaises(p.Refused):
                self.verify(result, version)

    def test_migration66_defaults_must_hold_in_every_old_row(self):
        # alpha.128: 66 adds NOT NULL columns with a fixed default to the old
        # service_plans and subscriptions; an old row holding anything else
        # was changed after the migration and is refused.
        self.assertEqual(p.added_default_columns(65), {})
        self.assertEqual(sorted(p.added_default_columns(66)), ['service_plans', 'subscriptions'])
        result = self.migrated_to(66)
        self.verify(result, 66)
        connection = sqlite3.connect(result['database'])
        connection.execute("UPDATE subscriptions SET overuse_policy='notify' WHERE id=83")
        connection.commit()
        connection.close()
        with self.assertRaises(p.Refused):
            self.verify(result, 66)

    def test_schema42_copy_is_not_accepted_as_schema43(self):
        result = self.build()
        connection = sqlite3.connect(result['database'])
        connection.execute('PRAGMA foreign_keys=ON')
        apply(connection, self.sql[38:42])
        connection.close()
        with self.assertRaises(p.Refused):
            self.verify(result, 43)

    def test_old_row_modification_and_deletion_are_both_refused(self):
        for sql in ("UPDATE users SET password_hash='changed' WHERE id=81", 'DELETE FROM domain_aliases WHERE id=87'):
            with self.subTest(sql=sql):
                result = self.build()
                connection = sqlite3.connect(result['database'])
                connection.execute(sql)
                connection.commit()
                connection.close()
                with self.assertRaises(p.Refused):
                    self.verify(result)

    def test_existing_sequence_counter_is_not_excluded_from_verification(self):
        result = self.build()
        connection = sqlite3.connect(result['database'])
        connection.execute("INSERT INTO users(username,password_hash,email,role) VALUES('later','!','later@example.test','customer')")
        connection.commit()
        connection.close()
        # sqlite_sequence is existing old state too: unapproved advancement is
        # reported as a changed old row, never silently excluded from proof.
        with self.assertRaisesRegex(p.Refused, 'sqlite_sequence'):
            self.verify(result)

    def test_appended_metrics_reported_and_prior_typed_row_is_still_verified(self):
        result = self.build()
        connection = sqlite3.connect(result['database'])
        connection.execute("INSERT INTO metrics_samples VALUES('2000-01-01T00:00:30Z',2.25,124,999,457,888,0.85)")
        connection.commit()
        connection.close()
        proof = self.verify(result)
        self.assertEqual(proof['tables']['metrics_samples'], {'before': 1, 'after': 2, 'added': 1})
        self.assertFalse(proof['global_bytes_equal_to_populated'])
        connection = sqlite3.connect(result['database'])
        connection.execute('UPDATE metrics_samples SET cpu=9 WHERE rowid=1')
        connection.commit()
        connection.close()
        with self.assertRaisesRegex(p.Refused, 'metrics_samples'):
            self.verify(result)

    def test_actual_builder_sigkill_preserves_source_and_unfinished_output(self):
        child = os.fork()
        if child == 0:
            def kill_after_write(connection, nonce):
                connection.execute("INSERT INTO users(username,password_hash,email,role) VALUES('partial','!','partial@example.test','customer')")
                os.kill(os.getpid(), signal.SIGKILL)
            p._insert_fixture = kill_after_write
            try:
                self.build()
            except BaseException:
                os._exit(91)
            os._exit(92)
        waited, status = os.waitpid(child, 0)
        self.assertEqual(waited, child)
        self.assertTrue(os.WIFSIGNALED(status))
        self.assertEqual(os.WTERMSIG(status), signal.SIGKILL)
        directory, = self.output.iterdir()
        self.assertFalse((directory / 'manifest.json').exists())
        self.assertTrue((directory / 'admission.json').is_file())
        self.assertTrue((directory / 'celikpanel.db-journal').is_file())
        with self.assertRaisesRegex(p.Refused, 'sidecar'):
            p._read_private(directory / 'celikpanel.db')
        self.assertEqual(self.source.read_bytes(), self.baseline_bytes)

    def test_newly_sealed_malformed_manifest_or_omitted_columns_still_refused(self):
        result = self.build()
        for change in ('columns', 'duplicate-row', 'malformed-column', 'wrong-ids', 'bad-hash'):
            with self.subTest(change=change):
                modified = json.loads(encoded(result['manifest']))
                if change == 'columns':
                    modified['tables']['users']['columns'].remove('totp_secret')
                elif change == 'duplicate-row':
                    row = modified['tables']['users']['rows'][0]
                    modified['tables']['users']['rows'].append(row)
                    modified['tables']['users']['count'] += 1
                elif change == 'malformed-column':
                    modified['tables']['users']['columns'][0] = []
                elif change == 'wrong-ids':
                    modified['fixture_ids'] = None
                else:
                    modified['populated_sha256'] = None
                with self.assertRaises(p.Refused):
                    p.verify_copy(Path(result['database']), modified, manifest_sha256=digest(encoded(modified)), expected_version=38)

    def test_manifest_tamper_is_refused_before_reading_database(self):
        result = self.build()
        result['manifest']['tables']['users']['rows'] = []
        with self.assertRaisesRegex(p.Refused, 'sealed manifest'):
            self.verify(result)

    def test_type_preserving_row_hash_distinguishes_blob_text_null_float_integer(self):
        values = [b'1', '1', None, 1, 1.0]
        self.assertEqual(len({p._row_digest([1, v]) for v in values}), len(values))

    def test_wrong_source_digest_or_admission_refuses_without_creating_output(self):
        for key, value in (('source_sha256', '0' * 64), ('baseline_commit', 'b' * 40),
                           ('baseline_migrations_sha256', 'f' * 64), ('nonce', '../escape'),
                           ('purpose', 'install-on-live-server')):
            with self.subTest(key=key):
                admission = dict(self.admission, **{key: value})
                with self.assertRaises(p.Refused):
                    p.build_private_copy(self.source, self.output, admission)
                self.assertEqual(list(self.output.iterdir()), [])

    def test_exact38_ledger_hash_filename_and_coverage_are_required(self):
        for sql in ("UPDATE schema_migrations SET sha256='bad' WHERE version=38",
                    "UPDATE schema_migrations SET filename='038_fake.sql' WHERE version=38",
                    'DELETE FROM schema_migrations WHERE version=38'):
            with self.subTest(sql=sql):
                self.source.write_bytes(self.baseline_bytes)
                self.change_source(sql)
                before = self.source.read_bytes()
                with self.assertRaises(p.Refused):
                    self.build()
                self.assertEqual(self.source.read_bytes(), before)
                self.assertTrue(all(not (d / 'manifest.json').exists() for d in self.output.iterdir()))

    def test_ledger_spoof_with_modified_trigger_is_refused(self):
        self.change_source('DROP TRIGGER trg_domains_hostname_reserve_insert')
        with self.assertRaisesRegex(p.Refused, 'schema SQL'):
            self.build()

    def test_other_schema_and_unknown_objects_refused(self):
        for alteration in ('CREATE TABLE owner_extension(id INTEGER)', 'DROP INDEX idx_users_email'):
            with self.subTest(alteration=alteration):
                self.source.write_bytes(self.baseline_bytes)
                self.change_source(alteration)
                with self.assertRaisesRegex(p.Refused, 'schema SQL'):
                    self.build()

    def test_real_schema39_is_not_admitted_as_old38(self):
        connection = sqlite3.connect(self.source)
        apply(connection, self.sql[38:39])
        connection.close()
        self.admission['source_sha256'] = digest(self.source.read_bytes())
        with self.assertRaises(p.Refused):
            self.build()

    def test_existing_hostname_user_and_dns_conflicts_are_preserved(self):
        names = p._names(self.admission['nonce'])
        mutations = [
            ('INSERT INTO domain_aliases(domain_id,alias) VALUES(85,?)', (names['main'],)),
            ('INSERT INTO users(username,password_hash,email,role) VALUES(?,?,?,?)',
             (names['username'], '!', names['email'], 'customer')),
            ("INSERT INTO pdns_domains(name,type) VALUES(?,'NATIVE')", (names['main'],)),
        ]
        for sql, args in mutations:
            with self.subTest(sql=sql):
                self.source.write_bytes(self.baseline_bytes)
                self.change_source(sql, args)
                before = self.source.read_bytes()
                with self.assertRaises(p.Refused):
                    self.build()
                self.assertEqual(self.source.read_bytes(), before)
                self.assertTrue(all(not (d / 'manifest.json').exists() for d in self.output.iterdir()))

    def test_sql_failure_after_first_insert_rolls_back_private_copy_only(self):
        def fail_after_write(connection, nonce):
            connection.execute("INSERT INTO users(username,password_hash,email,role) VALUES('partial','!','partial@example.test','customer')")
            raise p.Refused('injected fixture-only failure')
        with mock.patch.object(p, '_insert_fixture', fail_after_write):
            with self.assertRaises(p.Refused):
                self.build()
        directory, = self.output.iterdir()
        self.assertFalse((directory / 'manifest.json').exists())
        connection = sqlite3.connect((directory / 'celikpanel.db').as_uri() + '?mode=ro&immutable=1', uri=True)
        self.assertEqual(connection.execute("SELECT count(*) FROM users WHERE username='partial'").fetchone(), (0,))
        connection.close()
        self.assertEqual(self.source.read_bytes(), self.baseline_bytes)

    def test_source_is_never_opened_with_sqlite(self):
        original = sqlite3.connect
        opened = []
        def watched(path, *args, **kwargs):
            opened.append(str(path))
            self.assertNotIn(str(self.source), str(path))
            self.assertIn(str(self.output), str(path))
            return original(path, *args, **kwargs)
        with mock.patch.object(p.sqlite3, 'connect', watched):
            self.build()
        self.assertGreaterEqual(len(opened), 3)

    def test_all_sidecars_even_symlinks_refuse_source_and_verifier(self):
        result = self.build()
        for source in (self.source, Path(result['database'])):
            for suffix in p.SIDECARS:
                with self.subTest(source=source.name, suffix=suffix):
                    sidecar = Path(str(source) + suffix)
                    sidecar.symlink_to('/nonexistent-private-evidence')
                    try:
                        with self.assertRaises(p.Refused):
                            self.build() if source == self.source else self.verify(result)
                        self.assertTrue(sidecar.is_symlink())
                    finally:
                        sidecar.unlink()

    def test_symlink_hardlink_and_permissive_source_metadata_refused(self):
        alias = self.root / 'alias.db'
        alias.symlink_to(self.source)
        with self.assertRaises((p.Refused, OSError)):
            p.build_private_copy(alias, self.output, self.admission)
        alias.unlink()
        os.link(self.source, alias)
        with self.assertRaises(p.Refused):
            self.build()
        alias.unlink()
        self.source.chmod(0o640)
        with self.assertRaises(p.Refused):
            self.build()
        self.assertEqual(list(self.output.iterdir()), [])

    def test_private_parent_and_explicit_product_output_refusal(self):
        self.output.chmod(0o755)
        with self.assertRaises(p.Refused):
            self.build()
        self.output.chmod(0o700)
        for forbidden in ('/var/lib/celikpanel', '/opt/celikpanel/test', '/run/celikpanel/test'):
            with self.subTest(forbidden=forbidden), self.assertRaisesRegex(p.Refused, 'product paths'):
                p.build_private_copy(self.source, Path(forbidden), self.admission)
        self.root.chmod(0o755)
        try:
            with self.assertRaises(p.Refused):
                self.build()
        finally:
            self.root.chmod(0o700)

    def test_verifier_is_read_only_and_retains_file_identity(self):
        result = self.build()
        database = Path(result['database'])
        before = p._identity(database.stat())
        raw = database.read_bytes()
        self.verify(result)
        self.assertEqual(p._identity(database.stat()), before)
        self.assertEqual(database.read_bytes(), raw)
        self.assertFalse(any(Path(str(database) + s).exists() for s in p.SIDECARS))

    def test_wrong_migrated_defaults_refuse_even_if_only_new_column_changed(self):
        result = self.build()
        connection = sqlite3.connect(result['database'])
        apply(connection, self.sql[38:42])
        # The real trigger itself refuses this owner-policy rewrite. A fake
        # direct ALTER/trigger deletion also fails the pinned schema proof.
        with self.assertRaises(sqlite3.IntegrityError):
            connection.execute("UPDATE domains SET dns_management='external' WHERE id=85")
        connection.rollback()
        connection.execute('DROP TRIGGER domain_dns_management_immutable')
        connection.execute("UPDATE domains SET dns_management='external' WHERE id=85")
        connection.commit()
        connection.close()
        with self.assertRaises(p.Refused):
            self.verify(result, 42)

    def test_same_bytes_source_inode_replacement_before_final_proof_refused(self):
        original = p._insert_fixture
        def replace(connection, nonce):
            result = original(connection, nonce)
            replacement = self.root / 'replacement.db'
            replacement.write_bytes(self.baseline_bytes)
            replacement.chmod(0o600)
            os.replace(replacement, self.source)
            return result
        with mock.patch.object(p, '_insert_fixture', replace):
            with self.assertRaisesRegex(p.Refused, 'source changed'):
                self.build()
        self.assertTrue(all(not (d / 'manifest.json').exists() for d in self.output.iterdir()))


if __name__ == '__main__':
    unittest.main()
