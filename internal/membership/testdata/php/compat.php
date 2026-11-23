<?php
declare(strict_types=1);
// Test helper for internal/membership/php_parity_test.go only: answers, for a corpus
// written by the Go test, what PHP itself says: filter_var(FILTER_VALIDATE_EMAIL),
// portal-membership's Service::email() and Service::publicIP() (called as they are,
// through reflection), htmlspecialchars as view.php's h() calls it, and parse_str
// (the parser that fills $_GET and $_POST). Never deployed.
// usage: php compat.php <portal-membership/src/Service.php> <input.json>

require $argv[1];

use CelikPanel\Membership\Problem;
use CelikPanel\Membership\Service;

$input = json_decode((string)file_get_contents($argv[2]), true, 16, JSON_THROW_ON_ERROR);
$email = new ReflectionMethod(Service::class, 'email');
$publicIP = new ReflectionMethod(Service::class, 'publicIP');
$out = ['filter' => [], 'email' => [], 'ip' => [], 'html' => [], 'query' => []];
foreach ($input['emails'] as $encoded) {
    $value = (string)base64_decode((string)$encoded, true);
    $out['filter'][] = filter_var($value, FILTER_VALIDATE_EMAIL) !== false;
    try {
        $out['email'][] = base64_encode((string)$email->invoke(null, $value));
    } catch (Problem $e) {
        $out['email'][] = '!' . $e->getMessage();
    }
}
foreach ($input['ips'] as $value) {
    try {
        $out['ip'][] = (string)$publicIP->invoke(null, (string)$value);
    } catch (Problem $e) {
        $out['ip'][] = '!' . $e->getMessage();
    }
}
foreach ($input['html'] as $encoded) {
    $out['html'][] = base64_encode(htmlspecialchars((string)base64_decode((string)$encoded, true), ENT_QUOTES | ENT_SUBSTITUTE, 'UTF-8'));
}
foreach ($input['queries'] as $encoded) {
    $vars = [];
    parse_str((string)base64_decode((string)$encoded, true), $vars);
    $row = [];
    foreach ($vars as $name => $value) {
        $row[base64_encode((string)$name)] = is_array($value) ? 'a' : 's' . base64_encode((string)$value);
    }
    ksort($row, SORT_STRING);
    $out['query'][] = (object)$row;
}
$out['version'] = PHP_VERSION;
echo json_encode($out, JSON_THROW_ON_ERROR | JSON_UNESCAPED_SLASHES);
