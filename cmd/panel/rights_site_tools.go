package main

// The two permissions of alpha.118 (D-042, docs/PLANS-AND-RIGHTS.md section
// 4.2): WordPress tools (status, verified updates, staging copies) and the
// results of malware scans of the subscription's own sites. They join the
// closed permission set here so the rights model's own tables stay as they
// were; every set with "all permissions" (the Unlimited plans and plans that
// were migrated) includes them, a Custom set only when someone adds them.
//
// alpha.118 izinleri: WordPress araçları ve zararlı yazılım taraması.

const (
	rightWordPressTools = "wordpress_tools"
	rightMalwareScan    = "malware_scan"
)

func init() {
	rightPermissionIDs = append(rightPermissionIDs, rightWordPressTools, rightMalwareScan)
	rightPermissionNames[rightWordPressTools] = "WordPress tools"
	rightPermissionNames[rightMalwareScan] = "Malware scan"
}
