// Theme service for dark/light mode toggle
// Respects system preference, allows user override, persists in localStorage

export type Theme = 'light' | 'dark';

const STORAGE_KEY = 'netwatcher-theme';

class ThemeService {
    private currentTheme: Theme = 'light';
    private listeners: Set<(theme: Theme) => void> = new Set();

    constructor() {
        this.initialize();
    }

    private initialize(): void {
        // Check for stored preference first
        const stored = localStorage.getItem(STORAGE_KEY) as Theme | null;

        if (stored && (stored === 'light' || stored === 'dark')) {
            this.currentTheme = stored;
        } else {
            // Fall back to system preference
            this.currentTheme = this.getSystemPreference();
        }

        this.applyTheme();

        // Listen for system preference changes
        window.matchMedia('(prefers-color-scheme: dark)').addEventListener('change', (e) => {
            // Only auto-switch if user hasn't set a preference
            if (!localStorage.getItem(STORAGE_KEY)) {
                this.currentTheme = e.matches ? 'dark' : 'light';
                this.applyTheme();
                this.notifyListeners();
            }
        });
    }

    private getSystemPreference(): Theme {
        return window.matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light';
    }

    private applyTheme(): void {
        document.documentElement.setAttribute('data-theme', this.currentTheme);
    }

    private notifyListeners(): void {
        this.listeners.forEach(listener => listener(this.currentTheme));
    }

    getTheme(): Theme {
        return this.currentTheme;
    }

    setTheme(theme: Theme): void {
        this.currentTheme = theme;
        localStorage.setItem(STORAGE_KEY, theme);
        this.applyTheme();
        this.notifyListeners();
    }

    toggle(): void {
        this.setTheme(this.currentTheme === 'light' ? 'dark' : 'light');
    }

    // Reset to system preference
    resetToSystem(): void {
        localStorage.removeItem(STORAGE_KEY);
        this.currentTheme = this.getSystemPreference();
        this.applyTheme();
        this.notifyListeners();
    }

    onThemeChange(listener: (theme: Theme) => void): () => void {
        this.listeners.add(listener);
        return () => this.listeners.delete(listener);
    }
}

export const themeService = new ThemeService();
