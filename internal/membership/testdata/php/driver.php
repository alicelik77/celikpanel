<?php
declare(strict_types=1);
// Test driver for internal/membership/php_parity_test.go only: runs
// portal-membership's own Service on a database with a fixed clock and prints, as
// JSON, what every operation answered. Never deployed and not part of
// portal-membership; it adds no behaviour of its own.
// usage: php driver.php <portal-membership/src/Service.php> <input.json>

require $argv[1];

use CelikPanel\Membership\Problem;
use CelikPanel\Membership\Service;

function driverDump(PDO $db): array
{
    $out = [];
    foreach (['members', 'tokens', 'licenses', 'events', 'rate_limits'] as $table) {
        $columns = $db->query('PRAGMA table_info(' . $table . ')')->fetchAll(PDO::FETCH_COLUMN, 1);
        $select = [];
        foreach ($columns as $column) {
            $select[] = 'quote(' . $column . ') AS ' . $column;
        }
        $out[$table] = $db->query('SELECT ' . implode(',', $select) . ' FROM ' . $table . ' ORDER BY rowid')->fetchAll(PDO::FETCH_ASSOC);
    }
    return $out;
}

$input = json_decode((string)file_get_contents($argv[2]), true, 64, JSON_THROW_ON_ERROR);
$now = (int)$input['now'];
$mail = [];
$service = new Service(
    (string)$input['db'],
    (string)base64_decode((string)$input['secret'], true),
    function (string $email, string $purpose, string $token) use (&$mail): void {
        $mail[] = ['email' => $email, 'purpose' => $purpose, 'token' => $token];
    },
    function () use (&$now): int {
        return $now;
    }
);
$results = [];
foreach ($input['ops'] as $op) {
    if (isset($op['now'])) {
        $now = (int)$op['now'];
    }
    $args = [];
    foreach (($op['args'] ?? []) as $arg) {
        if (is_string($arg) && preg_match('/^@(\d+)\.([a-z_]+)$/D', $arg, $m) === 1) {
            $arg = (string)($results[(int)$m[1]]['result'][$m[2]] ?? '');
        }
        $args[] = $arg;
    }
    try {
        switch ($op['op']) {
            case 'register': $service->register($args[0]); $result = null; break;
            case 'request': $service->requestToken($args[0], $args[1]); $result = null; break;
            case 'consume': $service->consumeToken($args[0], $args[1], $args[2]); $result = null; break;
            case 'login': $result = $service->login($args[0], $args[1]); break;
            case 'member': $result = $service->member((int)$args[0], (int)$args[1]); break;
            case 'licenses': $result = $service->licenses((int)$args[0]); break;
            case 'issue': $result = $service->issue((int)$args[0]); break;
            case 'reveal': $result = $service->reveal((int)$args[0], $args[1], $args[2]); break;
            case 'rotate': $result = $service->rotate((int)$args[0], $args[1], $args[2]); break;
            case 'release': $service->release((int)$args[0], $args[1], $args[2]); $result = null; break;
            case 'remove': $service->remove((int)$args[0], $args[1], $args[2]); $result = null; break;
            case 'renew': $service->renew((int)$args[0], $args[1], $args[2]); $result = null; break;
            case 'activate': $result = $service->activate($args[0], $args[1], $args[2], $args[3]); break;
            case 'refresh': $result = $service->refresh($args[0], $args[1], $args[2], $args[3]); break;
            case 'limit': $service->limit($args[0], (int)$args[1], (int)$args[2]); $result = null; break;
            case 'mail': $result = $mail; $mail = []; break;
            case 'sql': $service->db->exec($args[0]); $result = null; break;
            case 'dump': $result = driverDump($service->db); break;
            case 'schema': $result = $service->db->query('SELECT type,name,tbl_name,sql FROM sqlite_master ORDER BY name')->fetchAll(PDO::FETCH_ASSOC); break;
            default: throw new RuntimeException('unknown driver operation');
        }
        $results[] = ['ok' => true, 'result' => $result];
    } catch (Problem $e) {
        $results[] = ['ok' => false, 'code' => $e->getMessage()];
    }
}
echo json_encode($results, JSON_THROW_ON_ERROR | JSON_UNESCAPED_SLASHES);
