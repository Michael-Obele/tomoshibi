<script lang="ts">
import DocRenderer from '$lib/components/blocks/doc-renderer.svelte';

let { data } = $props();
</script>

<DocRenderer page={data.page} />
