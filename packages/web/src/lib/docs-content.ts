import {
	Play,
	Zap,
	Map,
	Code,
	Cpu,
	Grid,
	Lock,
	BookOpen,
	Palette,
	ArrowRight,
	CheckCircle,
	Download,
	Terminal,
	FileText,
	Database,
	Globe,
	Layers,
	Shield,
	Search,
	Eye,
	Settings,
	Copy,
	BarChart3,
	Clock,
	RefreshCw,
	Moon
} from '@lucide/svelte';
import type { Component } from 'svelte';
import type { Icon as IconType } from '@lucide/svelte';

export type DocSection =
	| { type: 'hero'; title: string; description: string }
	| { type: 'text'; content: string }
	| {
			type: 'alert';
			title: string;
			description: string;
			icon?: typeof IconType;
			variant?: 'default' | 'destructive';
	  }
	| {
			type: 'cards';
			items: { title: string; description: string; icon?: typeof IconType; badge?: string }[];
			columns?: number;
	  }
	| { type: 'steps'; items: { title: string; description: string; code?: string; note?: string }[] }
	| { type: 'code'; code: string; language?: string; title?: string }
	| { type: 'heading'; level: 2 | 3; text: string; icon?: typeof IconType; iconColor?: string }
	| { type: 'flow'; items: { title: string; description: string }[] }
	| {
			type: 'colors';
			items: { name: string; value: string; description: string; class?: string }[];
	  }
	| {
			type: 'features';
			items: { title: string; description: string; icon?: typeof IconType; features: string[] }[];
	  }
	| { type: 'tree'; content: string; note?: string }
	| { type: 'list'; items: string[]; title?: string }
	| { type: 'key-value'; items: { key: string; value: string; badge?: string }[] };

export type DocPage = {
	title: string;
	description: string;
	sections: DocSection[];
};

export const hero = {
	title: 'Cinder Frontend Documentation',
	description:
		'Welcome to the official documentation for the Cinder Frontend. A modern, type-safe web interface for the Cinder Scraper backend built with SvelteKit 5, Svelte 5 Runes, and Tailwind CSS v4.'
};

export const quickStart = [
	{
		title: 'Try Playground',
		description: 'Test scraping, crawling, and search interactively with your backend.',
		href: '/playground',
		icon: Play,
		iconColor: 'text-primary'
	},
	{
		title: 'Setup Guide',
		description: 'Get the frontend up and running in minutes.',
		href: '/docs/setup',
		icon: Zap,
		iconColor: 'text-amber-500'
	},
	{
		title: 'Architecture',
		description: 'Understand the technical design and data flow.',
		href: '/docs/architecture',
		icon: Map,
		iconColor: 'text-blue-500'
	}
];

export const techStack = [
	{
		title: 'Frontend',
		icon: Code,
		items: [
			{ name: 'SvelteKit 5', badge: 'Framework', variant: 'default' },
			{ name: 'Svelte 5 Runes', badge: 'Reactivity', variant: 'secondary' },
			{ name: 'Tailwind CSS v4', badge: 'Styling', variant: 'secondary' }
		]
	},
	{
		title: 'Backend Communication',
		icon: Cpu,
		items: [
			{ name: 'Remote Functions', badge: 'RPC', variant: 'default' },
			{ name: 'Valibot', badge: 'Validation', variant: 'secondary' },
			{ name: 'TypeScript', badge: 'Type Safe', variant: 'secondary' }
		]
	},
	{
		title: 'UI Components',
		icon: Grid,
		items: [
			{ name: 'shadcn-svelte', badge: 'Primitives', variant: 'default' },
			{ name: 'Bits UI', badge: 'Headless', variant: 'secondary' },
			{ name: 'Lucide Icons', badge: 'Icons', variant: 'secondary' }
		]
	},
	{
		title: 'Security',
		icon: Lock,
		items: [
			{ name: 'Private API Keys', badge: 'Server-side', variant: 'default' },
			{ name: 'No Trackers', badge: 'Privacy', variant: 'secondary' },
			{ name: 'Type Safety', badge: 'Validation', variant: 'secondary' }
		]
	}
];

export const principles = [
	{
		title: 'Modern Development',
		description:
			'Built with Svelte 5 runes, Remote Functions, and type-safe patterns for a delightful developer experience.',
		icon: Zap,
		highlight: true
	},
	{
		title: 'Open Source First',
		description: 'Easy to self-host with minimal configuration. Deploy anywhere with zero lock-in.',
		highlight: false
	},
	{
		title: 'Privacy Focused',
		description:
			'No external trackers. Direct communication with your backend. Your data stays yours.',
		highlight: false
	},
	{
		title: 'Lightning Fast',
		description:
			'Optimized bundle size, lazy loading, and efficient state management for snappy performance.',
		highlight: false
	},
	{
		title: 'Developer Friendly',
		description:
			'Clean code with TypeScript, well-documented, and following modern Svelte 5 patterns.',
		highlight: false
	}
];

export const documentationSections = [
	{
		title: 'Getting Started',
		description: 'Introduction and setup guide',
		href: '/docs/setup',
		icon: BookOpen
	},
	{
		title: 'Architecture Design',
		description: 'Technical design, data flow, and patterns',
		href: '/docs/architecture',
		icon: Map
	},
	{
		title: 'Features Overview',
		description: 'Scrape, Crawl, and Search capabilities',
		href: '/docs/features',
		icon: Grid
	},
	{
		title: 'User Flows',
		description: 'How users interact with the app',
		href: '/docs/user-flow',
		icon: Play
	},
	{
		title: 'Theme & Styling',
		description: 'Customize colors and design tokens',
		href: '/docs/theme',
		icon: Palette
	}
];

export const pages: Record<string, DocPage> = {
	setup: {
		title: 'Setup Guide',
		description: 'Get Cinder up and running in minutes with this comprehensive setup guide.',
		sections: [
			{
				type: 'alert',
				title: 'Prerequisites',
				description: 'Node.js 18+ or higher, Bun 1.0+, and a running Cinder Go backend instance.',
				icon: Zap
			},
			{
				type: 'heading',
				level: 2,
				text: 'Installation'
			},
			{
				type: 'steps',
				items: [
					{
						title: 'Clone the Repository',
						description: 'Get the latest source code from GitHub.',
						code: 'git clone https://github.com/Michael-Obele/cinder-sv.git\ncd cinder-sv'
					},
					{
						title: 'Install Dependencies',
						description: 'Use Bun for faster installation and better performance.',
						code: 'bun install',
						note: 'Note: If using npm or pnpm, adjust the commands accordingly.'
					},
					{
						title: 'Configure Environment Variables',
						description:
							'Copy the example environment file and update it with your backend details.',
						code: 'cp .env.example .env.local\n\n# Edit .env.local with:\nPRIVATE_CINDER_BACKEND_URL=http://localhost:8000\nPRIVATE_CINDER_API_KEY=your-api-key-here'
					},
					{
						title: 'Run Development Server',
						description: 'Start the local development server with hot-reloading.',
						code: 'bun run dev'
					}
				]
			},
			{
				type: 'heading',
				level: 2,
				text: 'Next Steps'
			},
			{
				type: 'cards',
				columns: 2,
				items: [
					{
						title: 'Try the Playground',
						description: 'Test your connection by scraping a URL in the playground.',
						icon: Play
					},
					{
						title: 'Read Architecture',
						description: 'Learn how the frontend communicates with the backend.',
						icon: Map
					}
				]
			}
		]
	},
	architecture: {
		title: 'Architecture Design',
		description: "Understanding the Cinder Frontend's technical foundation and data flow.",
		sections: [
			{
				type: 'heading',
				level: 2,
				text: 'High-Level Overview'
			},
			{
				type: 'text',
				content:
					'The Cinder Frontend is a SvelteKit application that interacts with the Cinder Go backend through a secure, type-safe RPC-like interface powered by Remote Functions.'
			},
			{
				type: 'heading',
				level: 3,
				text: 'Data Flow Pipeline',
				icon: Layers
			},
			{
				type: 'flow',
				items: [
					{
						title: 'User Interaction',
						description: 'User enters a URL or query in a Svelte 5 component.'
					},
					{
						title: 'Remote Function Call',
						description: 'Component calls a function imported from src/remote/cinder.remote.ts'
					},
					{
						title: 'Server-Side Execution',
						description:
							'SvelteKit executes the function on the server, retrieving private environment variables for secure API communication.'
					},
					{
						title: 'Backend Request',
						description:
							'The server makes a fetch call to the Go backend with the API key attached.'
					},
					{
						title: 'Response Serialization',
						description: 'SvelteKit serializes the response and returns it directly to the browser.'
					},
					{
						title: 'Reactive UI Update',
						description:
							'The UI reactively updates using Svelte 5 runes ($state, $derived, $effect).'
					}
				]
			},
			{
				type: 'heading',
				level: 2,
				text: 'Technology Stack'
			},
			{
				type: 'cards',
				columns: 2,
				items: [
					{
						title: 'Frontend Framework',
						description: 'Full-stack meta-framework with remote functions support.',
						icon: Code,
						badge: 'SvelteKit 5'
					},
					{
						title: 'Reactivity',
						description: '$state, $derived, $effect, $props for fine-grained reactivity.',
						icon: Zap,
						badge: 'Svelte 5 Runes'
					},
					{
						title: 'Styling',
						description: 'Utility-first CSS with OKLCH color space & Cinder Glow theme.',
						icon: Globe,
						badge: 'Tailwind CSS v4'
					},
					{
						title: 'Components',
						description: 'Headless component primitives with Tailwind styling.',
						icon: Database,
						badge: 'shadcn-svelte'
					},
					{
						title: 'Validation',
						description: 'Type-safe schema validation for all remote function inputs.',
						icon: Lock,
						badge: 'Valibot'
					},
					{
						title: 'API Communication',
						description: 'Type-safe server-side RPC calls with automatic serialization.',
						icon: Shield,
						badge: 'Remote Functions'
					}
				]
			},
			{
				type: 'heading',
				level: 2,
				text: 'Svelte 5 Runes & Remote Functions'
			},
			{
				type: 'alert',
				title: 'Modern Reactivity',
				description:
					'All components use Svelte 5 runes for fine-grained reactivity. Legacy patterns like let, $:, and export let are not used.',
				icon: Zap
			},
			{
				type: 'code',
				title: '$state - Component State',
				code: "let url = $state('');\nlet isLoading = $state(false);"
			},
			{
				type: 'code',
				title: '$derived - Computed Values',
				code: "let isValid = $derived(url.startsWith('http'));\nlet errorCount = $derived(errors.length);"
			},
			{
				type: 'code',
				title: '$effect - Side Effects',
				code: '$effect(() => {\n  if (crawlForm.result?.id) {\n    const interval = setInterval(() => {\n      statusQuery.refetch(crawlForm.result.id);\n    }, 2000);\n    return () => clearInterval(interval);\n  }\n});'
			},
			{
				type: 'code',
				title: 'Remote Functions - API Calls',
				code: '// Query pattern - for reading data\nlet statusQuery = getCrawlStatus();\n\n// Form pattern - for mutations\nlet scrapeForm = scrapeUrl();\n<form method="POST" use:scrapeForm.enhance>\n  <input {...scrapeForm.fields.url.as(\'text\')} />\n</form>'
			},
			{
				type: 'heading',
				level: 2,
				text: 'Directory Structure'
			},
			{
				type: 'tree',
				content:
					'/src\n├── lib/\n│   ├── components/        # Reusable UI components\n│   │   ├── ui/           # shadcn-svelte primitives\n│   │   └── blocks/       # Composed feature blocks\n│   └── utils.ts          # Helper functions\n├── remote/\n│   └── cinder.remote.ts  # Server-side RPC functions\n└── routes/\n    ├── +layout.svelte    # Global layout\n    ├── playground/       # Main feature page\n    └── docs/            # Documentation pages',
				note: 'All backend communication flows through src/remote/cinder.remote.ts, which handles API calls, validation, and type safety.'
			},
			{
				type: 'heading',
				level: 2,
				text: 'Security & Environment Variables'
			},
			{
				type: 'alert',
				title: 'Secrets Protection',
				description:
					'API keys and authentication credentials are stored in private environment variables and never exposed to the browser.',
				icon: Shield,
				variant: 'destructive'
			},
			{
				type: 'key-value',
				items: [
					{
						key: 'PRIVATE_CINDER_BACKEND_URL',
						value: 'Backend service endpoint (server-side only)'
					},
					{
						key: 'PRIVATE_CINDER_API_KEY',
						value: 'Authentication key for backend (server-side only)'
					}
				]
			},
			{
				type: 'list',
				title: 'Data Flow Security',
				items: [
					'API keys remain on the server, never sent to the client',
					'All backend communication happens server-side',
					'Results are serialized and returned to the browser',
					'No direct client-to-backend communication'
				]
			},
			{
				type: 'heading',
				level: 2,
				text: 'Design Principles'
			},
			{
				type: 'cards',
				columns: 2,
				items: [
					{
						title: 'Simplicity',
						description:
							'Minimal abstraction layers. Direct communication between UI and backend through type-safe remote functions.'
					},
					{
						title: 'Type Safety',
						description:
							'Full TypeScript support with Valibot schemas for validation. Compile-time and runtime checking.'
					},
					{
						title: 'Reactivity',
						description:
							'Fine-grained reactive state using Svelte 5 runes. Only what changes triggers updates.'
					},
					{
						title: 'Privacy First',
						description:
							'All sensitive operations stay server-side. No external trackers or analytics.'
					}
				]
			}
		]
	},
	features: {
		title: 'Features',
		description: 'Explore the powerful capabilities of the Cinder Frontend.',
		sections: [
			{
				type: 'heading',
				level: 2,
				text: 'Universal Playground'
			},
			{
				type: 'text',
				content:
					'The core feature of Cinder is a unified interface where users can test all scraping capabilities in one place. Switch between Scrape, Crawl, and Search operations with a single interface.'
			},
			{
				type: 'alert',
				title: 'Unified Experience',
				description:
					'All operations share the same core interface for options, output rendering, and results management. Learn once, use everywhere.',
				icon: Zap
			},
			{
				type: 'heading',
				level: 2,
				text: 'Scrape Feature',
				icon: Globe,
				iconColor: 'text-amber-500'
			},
			{
				type: 'text',
				content:
					'Instantly convert any URL into clean Markdown, HTML, or JSON. Perfect for quick content extraction and one-off website analysis.'
			},
			{
				type: 'features',
				items: [
					{
						title: 'Input & Validation',
						description: '',
						icon: Zap,
						features: [
							'URL input field with client-side validation',
							'Valibot schema for type safety',
							'Real-time validation feedback',
							'Support for HTTP and HTTPS protocols'
						]
					},
					{
						title: 'Advanced Options',
						description: '',
						icon: Settings,
						features: [
							'Custom headers configuration',
							'Cookie management',
							'Exclude specific tags',
							'JavaScript rendering toggle (static vs dynamic)'
						]
					},
					{
						title: 'Output Formats',
						description: '',
						icon: Eye,
						features: [
							'Markdown format (human-readable)',
							'HTML output (preserves structure)',
							'Raw JSON response',
							'Status code and metadata'
						]
					},
					{
						title: 'Result Handling',
						description: '',
						icon: Copy,
						features: [
							'One-click copy to clipboard',
							'Download as file (Markdown or JSON)',
							'Badge indicators for status',
							'Syntax highlighting for code'
						]
					}
				]
			},
			{
				type: 'list',
				title: 'Perfect for:',
				items: [
					'Quick content extraction',
					'SEO analysis',
					'Article downloading',
					'Data collection for analysis',
					'Testing website structure'
				]
			},
			{
				type: 'heading',
				level: 2,
				text: 'Crawl Feature',
				icon: BarChart3,
				iconColor: 'text-blue-500'
			},
			{
				type: 'text',
				content:
					'Deep domain crawling with automatic link discovery, progress tracking, and comprehensive results. Ideal for mapping entire websites and batch processing.'
			},
			{
				type: 'features',
				items: [
					{
						title: 'Job Control',
						description: '',
						icon: BookOpen,
						features: [
							'Start crawl with base URL',
							'Pause/resume operations',
							'Depth configuration',
							'URL filtering options'
						]
					},
					{
						title: 'Real-Time Monitoring',
						description: '',
						icon: Clock,
						features: [
							'Progress bar with percentage',
							'Live link discovery log',
							'Status badges (Success/Running/Failed)',
							'Auto-refresh with polling'
						]
					},
					{
						title: 'Results Export',
						description: '',
						icon: Download,
						features: [
							'All crawled links in structured format',
							'Metadata for each page',
							'CSV/JSON export options',
							'Downloadable report'
						]
					},
					{
						title: 'Polling System',
						description: '',
						icon: RefreshCw,
						features: [
							'Automatic status updates every 2s',
							'Efficient network usage',
							'Graceful cleanup on completion',
							'Smart error recovery'
						]
					}
				]
			},
			{
				type: 'list',
				title: 'Perfect for:',
				items: [
					'Website mapping and analysis',
					'Competitive intelligence',
					'Automated content discovery',
					'Link verification',
					'Large-scale data collection'
				]
			},
			{
				type: 'heading',
				level: 2,
				text: 'Search Feature',
				icon: Search,
				iconColor: 'text-green-500'
			},
			{
				type: 'text',
				content:
					'Combine web search with automatic content scraping. Enter a query, get ranked results, and automatically extract content from matching websites.'
			},
			{
				type: 'features',
				items: [
					{
						title: 'Query Input',
						description: '',
						icon: Search,
						features: [
							'Natural language search queries',
							'Advanced search operators',
							'Result limit configuration',
							'Real-time search suggestions'
						]
					},
					{
						title: 'Results Display',
						description: '',
						icon: BarChart3,
						features: [
							'Ranked search results',
							'Card layout for each result',
							'Title, URL, and snippet',
							'Metadata and relevance score'
						]
					},
					{
						title: 'Loading States',
						description: '',
						icon: Eye,
						features: [
							'Skeleton loaders while fetching',
							'Progressive content loading',
							'Error recovery',
							'Empty state handling'
						]
					},
					{
						title: 'Auto-Scraping',
						description: '',
						icon: CheckCircle,
						features: [
							'Click to scrape any result',
							'Background extraction',
							'Content preview toggle',
							'Multiple format support'
						]
					}
				]
			},
			{
				type: 'list',
				title: 'Perfect for:',
				items: [
					'Research and market analysis',
					'Competitor monitoring',
					'News aggregation',
					'Knowledge base building',
					'Content curation'
				]
			},
			{
				type: 'heading',
				level: 2,
				text: 'Cross-Feature Capabilities'
			},
			{
				type: 'cards',
				columns: 3,
				items: [
					{
						title: 'History Management',
						description:
							'Store and re-run previous requests from localStorage without database overhead.'
					},
					{
						title: 'Dark/Light Themes',
						description: 'Auto-detect system preference or manual toggle with persistent selection.'
					},
					{
						title: 'Responsive Design',
						description: 'Optimized for mobile, tablet, and desktop with touch-friendly controls.'
					},
					{
						title: 'Error Handling',
						description: 'Comprehensive error messages with recovery suggestions and retry options.'
					},
					{
						title: 'Accessibility',
						description: 'WCAG 2.1 AA compliant with keyboard navigation and screen reader support.'
					},
					{
						title: 'Performance',
						description: 'Optimized bundle size, lazy loading, and efficient state management.'
					}
				]
			},
			{
				type: 'heading',
				level: 2,
				text: 'Roadmap'
			},
			{
				type: 'alert',
				title: 'Coming Soon',
				description:
					'Advanced features like batch processing, scheduled crawls, API key management, and custom workflows are planned for future releases.',
				icon: Zap
			}
		]
	},
	theme: {
		title: 'Theme & Styling',
		description: 'Customize the look and feel of the Cinder Frontend.',
		sections: [
			{
				type: 'heading',
				level: 2,
				text: 'Cinder Glow Color Palette'
			},
			{
				type: 'text',
				content:
					'The "Cinder Glow" theme combines neutral Slate grays with warm Amber and Orange accents, creating a modern, accessible color scheme.'
			},
			{
				type: 'colors',
				items: [
					{
						name: '--primary',
						value: 'Amber 500',
						description: 'Main accent',
						class: 'bg-amber-500'
					},
					{
						name: '--primary-foreground',
						value: 'Amber 100/950',
						description: 'Primary text',
						class: 'bg-amber-100 dark:bg-amber-950'
					},
					{
						name: '--background',
						value: 'Slate 950/50',
						description: 'Page background',
						class: 'bg-slate-950 dark:bg-slate-50'
					},
					{
						name: '--border',
						value: 'Slate 200/800',
						description: 'Border lines',
						class: 'border border-slate-200 bg-white dark:border-slate-800 dark:bg-slate-950'
					}
				]
			},
			{
				type: 'key-value',
				items: [
					{ key: '--primary', value: 'Main accent', badge: 'default' },
					{ key: '--secondary', value: 'Alternative action', badge: 'outline' },
					{ key: '--muted-foreground', value: 'Subtle text', badge: 'secondary' },
					{ key: '--destructive', value: 'Errors', badge: 'destructive' }
				]
			},
			{
				type: 'heading',
				level: 2,
				text: 'Dark & Light Modes',
				icon: Moon
			},
			{
				type: 'cards',
				columns: 2,
				items: [
					{
						title: 'Dark Mode (Default)',
						description:
							'✓ Primary dark theme with high contrast\n✓ Reduced eye strain for extended use\n✓ Professional appearance aligned with Firecrawl\n✓ Better for accessibility (WCAG AA compliant)'
					},
					{
						title: 'Light Mode',
						description:
							'✓ Clean, minimal aesthetic\n✓ Bright, readable colors\n✓ Perfect for daylight use\n✓ User can toggle based on preference'
					}
				]
			},
			{
				type: 'heading',
				level: 2,
				text: 'Configuration File'
			},
			{
				type: 'text',
				content:
					'All theme configuration is centralized in src/routes/layout.css. This file defines CSS custom properties, color variables, and utility classes.'
			},
			{
				type: 'code',
				language: 'css',
				code: "@import 'tailwindcss/theme';\n\n@layer theme {\n  --color-primary: oklch(55% 0.12 45);\n  --color-primary-foreground: white;\n  \n  --color-background: oklch(98% 0 0);\n  --color-foreground: oklch(2% 0 0);\n  \n  --color-muted: oklch(96% 0 0);\n  --color-muted-foreground: oklch(45% 0 0);\n  \n  --radius-sm: 4px;\n  --radius-md: 8px;\n  --radius-lg: 12px;\n}\n\n/* Dark mode */\n@media (prefers-color-scheme: dark) {\n  @layer theme {\n    --color-background: oklch(12% 0 0);\n    --color-foreground: oklch(98% 0 0);\n    --color-muted: oklch(16% 0 0);\n    --color-muted-foreground: oklch(64% 0 0);\n  }\n}"
			},
			{
				type: 'heading',
				level: 2,
				text: 'Customization Guide'
			},
			{
				type: 'cards',
				columns: 1,
				items: [
					{
						title: 'Changing Colors',
						description:
							'1. Open src/routes/layout.css\n2. Modify the OKLCH values in the @layer theme section\n3. Update both light and dark mode variants\n4. Rebuild and test in browser',
						icon: Code
					},
					{
						title: 'Preview Changes',
						description:
							'Use the dev server with hot reload to instantly see theme changes. No build required.',
						icon: Eye
					},
					{
						title: 'OKLCH Color Space',
						description:
							'OKLCH provides:\n✓ Perceptually uniform colors\n✓ Better color interpolation\n✓ More accessible contrast ratios\n✓ Modern browser support',
						icon: CheckCircle
					}
				]
			},
			{
				type: 'heading',
				level: 2,
				text: 'Semantic CSS Classes'
			},
			{
				type: 'text',
				content: 'Use these semantic Tailwind classes instead of hardcoded colors for consistency:'
			},
			{
				type: 'key-value',
				items: [
					{ key: 'bg-primary', value: 'Primary action backgrounds' },
					{ key: 'text-primary', value: 'Primary text (accents)' },
					{ key: 'bg-background', value: 'Page/component backgrounds' },
					{ key: 'text-foreground', value: 'Main body text' },
					{ key: 'bg-muted', value: 'Secondary backgrounds' },
					{ key: 'text-muted-foreground', value: 'Secondary text' },
					{ key: 'border-border', value: 'Border lines' }
				]
			},
			{
				type: 'heading',
				level: 2,
				text: 'Responsive Design Tokens'
			},
			{
				type: 'key-value',
				items: [
					{ key: 'sm', value: '640px' },
					{ key: 'md', value: '768px' },
					{ key: 'lg', value: '1024px' },
					{ key: 'xl', value: '1280px' }
				]
			},
			{
				type: 'heading',
				level: 2,
				text: 'Best Practices'
			},
			{
				type: 'alert',
				title: 'Theme Consistency',
				description:
					'Always use semantic color tokens instead of hardcoding HSL/Hex values. This ensures consistency across light/dark modes.',
				icon: Settings
			},
			{
				type: 'list',
				items: [
					'Use Tailwind utility classes for styling',
					'Leverage semantic color variables for theming',
					'Keep hardcoded colors to a minimum',
					'Test color contrast for accessibility (WCAG AA)',
					'Preview changes in both light and dark modes',
					'Use cn() utility for conditional class merging',
					'Prefer motion-safe utilities for animations'
				]
			},
			{
				type: 'heading',
				level: 2,
				text: 'Tools & Resources'
			},
			{
				type: 'cards',
				columns: 2,
				items: [
					{
						title: 'OKLCH Color Picker',
						description:
							'Use online tools to convert RGB/Hex colors to OKLCH format for use in CSS.'
					},
					{
						title: 'Contrast Checker',
						description: 'Verify color combinations meet WCAG AA accessibility standards.'
					},
					{
						title: 'Tailwind Docs',
						description: 'Reference official Tailwind CSS v4 documentation for the latest features.'
					},
					{
						title: 'Browser DevTools',
						description: 'Use browser inspector to debug CSS variables and view computed styles.'
					}
				]
			}
		]
	},
	'user-flow': {
		title: 'User Flow & Interaction Patterns',
		description:
			'Understanding how users interact with the Cinder Frontend to accomplish their goals.',
		sections: [
			{
				type: 'heading',
				level: 2,
				text: 'Scrape Flow',
				icon: Globe,
				iconColor: 'text-amber-500'
			},
			{
				type: 'text',
				content: 'The typical user journey when scraping a single URL.'
			},
			{
				type: 'flow',
				items: [
					{ title: 'Enter URL', description: 'User enters a URL in the input field.' },
					{
						title: 'Optional: Configure Options',
						description:
							'Click the Options button to open Sheet with headers, cookies, exclude tags, and rendering mode.'
					},
					{
						title: 'Click Scrape Button',
						description: 'Submits the form to the backend via remote function.'
					},
					{
						title: 'View Results',
						description: 'Results appear in tabbed interface: Markdown, HTML, JSON.'
					},
					{
						title: 'Copy or Download',
						description: 'User can copy to clipboard or download as file.'
					}
				]
			},
			{
				type: 'list',
				title: 'Key Interactions',
				items: [
					'Real-time URL validation',
					'Form progressive enhancement',
					'Instant result tabs',
					'Copy feedback (toast notification)'
				]
			},
			{
				type: 'heading',
				level: 2,
				text: 'Crawl Flow',
				icon: BarChart3,
				iconColor: 'text-blue-500'
			},
			{
				type: 'text',
				content: 'The user journey for deep domain crawling with job management.'
			},
			{
				type: 'flow',
				items: [
					{
						title: 'Enter Base URL',
						description: 'User enters the domain to crawl (e.g., https://example.com).'
					},
					{
						title: 'Configure Crawl Depth',
						description:
							'Optional: Set depth limit, URL patterns to include/exclude, and max pages.'
					},
					{
						title: 'Click Start Crawl',
						description: 'Initiates the crawl job. Backend returns a job ID.'
					},
					{
						title: 'Monitor Progress',
						description:
							'Live progress bar and scrollable log of discovered links. Auto-polls every 2 seconds.'
					},
					{
						title: 'Pause or Stop',
						description: 'User can pause the crawl to examine partial results or stop completely.'
					},
					{
						title: 'Download Results',
						description: 'Once complete, download the full crawl report as CSV or JSON.'
					}
				]
			},
			{
				type: 'list',
				title: 'Key Interactions',
				items: [
					'Async job management with polling',
					'Real-time progress visualization',
					'Live log with auto-scroll',
					'Status badges for each link'
				]
			},
			{
				type: 'heading',
				level: 2,
				text: 'Search Flow',
				icon: Search,
				iconColor: 'text-green-500'
			},
			{
				type: 'text',
				content:
					'Combine web search with automatic content scraping. Enter a query, get ranked results, and automatically extract content from matching websites.'
			},
			{
				type: 'flow',
				items: [
					{
						title: 'Enter Search Query',
						description: 'User enters natural language search query or advanced search operators.'
					},
					{
						title: 'Configure Search Options',
						description: 'Optional: Set result limit, language, region, time range.'
					},
					{ title: 'Click Search', description: 'Submits search query to backend web search API.' },
					{
						title: 'Browse Results',
						description: 'Search results appear as cards with title, URL, snippet, and metadata.'
					},
					{
						title: 'Scrape Individual Result',
						description: 'Click "Scrape" button on any result to extract full content.'
					},
					{
						title: 'View Extracted Content',
						description: 'Content appears in modal/drawer for review and export.'
					}
				]
			},
			{
				type: 'list',
				title: 'Key Interactions',
				items: [
					'Search suggestions and autocomplete',
					'Skeleton loaders while fetching',
					'Inline scrape without navigation',
					'Multiple result export formats'
				]
			},
			{
				type: 'heading',
				level: 2,
				text: 'History & Quick Access'
			},
			{
				type: 'text',
				content: 'Users can store and replay previous requests for efficiency.'
			},
			{
				type: 'alert',
				title: 'Local Storage',
				description:
					'History is stored in localStorage (not a database) for quick access without server overhead. Users can clear history anytime.',
				icon: Zap
			},
			{
				type: 'cards',
				columns: 2,
				items: [
					{
						title: 'History Sidebar',
						description:
							'Quick-access list of last 10-20 requests with one-click re-run capability.'
					},
					{
						title: 'Persistent State',
						description: 'Forms remember last input values for faster repeated queries.'
					}
				]
			},
			{
				type: 'heading',
				level: 2,
				text: 'Error Handling & Recovery'
			},
			{
				type: 'text',
				content:
					'The UI gracefully handles errors and provides users with actionable recovery steps.'
			},
			{
				type: 'cards',
				columns: 1,
				items: [
					{
						title: 'Network Errors',
						description: 'User sees error message with retry button. Form state is preserved.'
					},
					{
						title: 'Validation Errors',
						description:
							'Client-side validation shows errors below the input field. User can fix and resubmit.'
					},
					{
						title: 'Backend Errors',
						description: 'Specific error messages with HTTP status codes help debugging.'
					},
					{
						title: 'Timeout Recovery',
						description:
							'For long-running operations, timeout errors suggest increasing limits or retrying.'
					}
				]
			},
			{
				type: 'heading',
				level: 2,
				text: 'Accessibility & Navigation'
			},
			{
				type: 'cards',
				columns: 2,
				items: [
					{
						title: 'Keyboard Navigation',
						description:
							'✓ Tab to navigate through all controls\n✓ Enter to submit forms\n✓ Escape to close modals\n✓ Arrow keys for menus'
					},
					{
						title: 'Screen Reader Support',
						description:
							'✓ ARIA labels on all inputs\n✓ Semantic HTML structure\n✓ Alt text for icons\n✓ Form error announcements'
					}
				]
			}
		]
	}
};

export const documentationGroups = [
	{
		title: 'Getting Started',
		items: [
			{ title: 'Introduction', url: '/docs', icon: BookOpen },
			{ title: 'Setup Guide', url: '/docs/setup', icon: Zap }
		]
	},
	{
		title: 'Core Concepts',
		items: [
			{ title: 'Architecture', url: '/docs/architecture', icon: Map },
			{ title: 'Features', url: '/docs/features', icon: Grid },
			{ title: 'User Flow', url: '/docs/user-flow', icon: Terminal }
		]
	},
	{
		title: 'Reference',
		items: [{ title: 'Theme Config', url: '/docs/theme', icon: Shield }]
	}
];
