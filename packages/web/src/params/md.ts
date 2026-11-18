// `/docs/<slug>.md` is markdown; `/docs/<slug>` is the human page. The matcher
// is what keeps one out of the other's route slot.
export const match = (param: string): boolean => param.endsWith('.md');
