import { sections } from '$lib/server/agentdocs';

// The sidebar is the catalog, so a published document appears without a second
// edit somewhere in the UI.
export function load() {
	return { sections };
}
