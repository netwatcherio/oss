<template>
    <footer class="footer">
        <div class="container">
            <div class="footer-content">
                <div class="footer-brand">
                    <router-link to="/" class="logo">
                        <i class="bi bi-eye"></i>
                        <span>netwatcher</span>
                        <span class="badge">OSS</span>
                    </router-link>
                    <p>Open source network monitoring.<br>Built by engineers, for engineers.</p>
                    <div class="footer-social">
                        <a href="https://github.com/netwatcherio/oss" target="_blank" aria-label="GitHub">
                            <i class="bi bi-github"></i>
                        </a>
                        <a href="mailto:hello@netwatcher.io" aria-label="Email">
                            <i class="bi bi-envelope"></i>
                        </a>
                    </div>
                </div>
                <div class="footer-links">
                    <div class="footer-column">
                        <h4>Product</h4>
                        <router-link to="/#features">Features</router-link>
                        <router-link to="/#comparison">Compare</router-link>
                        <router-link to="/#roadmap">Roadmap</router-link>
                        <router-link to="/demo">Demo</router-link>
                    </div>
                    <div class="footer-column">
                        <h4>Resources</h4>
                        <a href="https://github.com/netwatcherio/oss/blob/master/docs/README.md"
                            target="_blank">Documentation</a>
                        <a href="https://github.com/netwatcherio/oss/blob/master/docs/api-reference.md"
                            target="_blank">API Reference</a>
                        <a href="https://github.com/netwatcherio/oss/issues" target="_blank">Report Issue</a>
                    </div>
                    <div class="footer-column">
                        <h4>Community</h4>
                        <a href="https://github.com/netwatcherio/oss" target="_blank">GitHub</a>
                        <a href="https://github.com/netwatcherio/oss/blob/master/CONTRIBUTING.md"
                            target="_blank">Contributing</a>
                        <a href="https://github.com/netwatcherio/oss/blob/master/LICENSE.md" target="_blank">License
                            (AGPL-3.0)</a>
                    </div>
                    <div class="footer-column">
                        <h4>Legal</h4>
                        <router-link to="/privacy">Privacy Policy</router-link>
                        <router-link to="/terms">Terms of Service</router-link>
                    </div>
                </div>
            </div>
            <div class="footer-bottom">
                <p>&copy; 2026 NetWatcher OSS. AGPL-3.0. Made with caffeine and spite for overpriced enterprise
                    software.</p>
            </div>
        </div>
    </footer>
</template>
