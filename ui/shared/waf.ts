export const WAF_DEFAULT_MODES = ['BLOCK', 'MONITOR'] as const;
export const WAF_MODES = ['INHERIT', ...WAF_DEFAULT_MODES, 'OFF'] as const;
export const WAF_CHANNELS = ['LTS', 'STABLE'] as const;

export type WAFDefaultMode = typeof WAF_DEFAULT_MODES[number];
export type WAFMode = typeof WAF_MODES[number];
export type WAFChannel = typeof WAF_CHANNELS[number];
