<template>
    <!-- Hero Section -->
    <header class="hero">
        <canvas id="network-canvas" ref="canvasRef" class="network-bg"></canvas>
        <div class="hero-glow"></div>
        <div class="container">
            <div class="hero-badge" id="typewriter-badge">
                <span class="badge-icon"><i class="bi bi-unlock"></i></span>
                <span class="typewriter-text" ref="typewriterRef"></span><span class="cursor">|</span>
            </div>
            <h1 class="hero-title">
                <span class="title-line">Stop Overpaying for</span>
                <span class="title-line gradient-text">Network Visibility</span>
            </h1>
            <p class="hero-subtitle">
                Your network doesn't care about enterprise contracts. Neither do we.
                <br>
                <span class="subtitle-highlight">Production-grade monitoring. Zero BS pricing.</span>
            </p>
            <div class="hero-actions">
                <a href="https://app.netwatcher.io" class="btn btn-primary btn-lg btn-glow" target="_blank">
                    <i class="bi bi-lightning-charge"></i>
                    <span>Launch Dashboard</span>
                    <span class="btn-arrow">→</span>
                </a>
                <router-link to="/#get-started" class="btn btn-outline btn-lg btn-dim">
                    <i class="bi bi-hdd-rack"></i>
                    <span>Self-Host</span>
                </router-link>
            </div>

            <!-- Live Stats Ticker -->
            <div class="stats-ticker">
                <div class="ticker-item">
                    <div class="ticker-icon"><i class="bi bi-infinity"></i></div>
                    <div class="ticker-content">
                        <span class="ticker-value" data-count="0" data-target="0" data-suffix="">$0</span>
                        <span class="ticker-label">Forever Free</span>
                    </div>
                </div>
                <div class="ticker-divider"></div>
                <div class="ticker-item">
                    <div class="ticker-icon"><i class="bi bi-hdd-network"></i></div>
                    <div class="ticker-content">
                        <span class="ticker-value" data-count="0" data-target="20" data-suffix="+">0</span>
                        <span class="ticker-label">Agents (Free Tier)</span>
                    </div>
                </div>
                <div class="ticker-divider"></div>
                <div class="ticker-item">
                    <div class="ticker-icon"><i class="bi bi-git"></i></div>
                    <div class="ticker-content">
                        <span class="ticker-value" data-count="0" data-target="100" data-suffix="%">0%</span>
                        <span class="ticker-label">Open Source</span>
                    </div>
                </div>
            </div>
        </div>
    </header>

    <!-- Pain Points Section -->
    <section class="section section-pain">
        <div class="container">
            <div class="pain-grid">
                <div class="pain-card">
                    <div class="pain-emoji">💸</div>
                    <h3>SaaS Tools: ~$300/mo</h3>
                    <p>That's $60 per agent. <em>Per month.</em> For doing what a script could do.</p>
                </div>
                <div class="pain-card">
                    <div class="pain-emoji">🏢</div>
                    <h3>Enterprise: "Call for pricing"</h3>
                    <p>Translation: $2,000+/mo and a sales team that won't stop calling.</p>
                </div>
                <div class="pain-card pain-solution">
                    <div class="pain-emoji">🎯</div>
                    <h3>NetWatcher: $0</h3>
                    <p>Same features. Same performance. Actually, probably better - you can read the code.</p>
                </div>
            </div>
        </div>
    </section>

    <!-- Features Section -->
    <section id="features" class="section">
        <div class="container">
            <div class="section-header">
                <span class="section-eyebrow">The Toolkit</span>
                <h2 class="section-title">Everything You Actually Need</h2>
                <p class="section-subtitle">No fluff. No "enterprise features" locked behind paywalls.<br>Just solid
                    monitoring that works.</p>
            </div>

            <div class="features-grid">
                <div class="feature-card feature-highlight">
                    <div class="feature-orb">
                        <div class="orb-pulse"></div>
                        <i class="bi bi-diagram-3"></i>
                    </div>
                    <h3>MTR That Makes Sense</h3>
                    <p>Native Go traceroutes with path analysis and route change detection. No more parsing text output
                        like it's 1995.</p>
                    <div class="feature-meta">
                        <span class="meta-tag"><i class="bi bi-check2"></i> Route changes</span>
                        <span class="meta-tag"><i class="bi bi-check2"></i> Hop analysis</span>
                    </div>
                </div>

                <div class="feature-card">
                    <div class="feature-orb orb-blue"><i class="bi bi-hdd-network"></i></div>
                    <h3>Agents Everywhere</h3>
                    <p>Windows and Linux. Deploy in minutes. Monitor from anywhere your users actually are.</p>
                </div>

                <div class="feature-card">
                    <div class="feature-orb orb-purple"><i class="bi bi-speedometer2"></i></div>
                    <h3>TrafficSim</h3>
                    <p>Synthetic UDP traffic with bidirectional analysis. Latency, jitter, loss, reordering - the full
                        picture.</p>
                </div>

                <div class="feature-card">
                    <div class="feature-orb orb-orange"><i class="bi bi-grid-3x3"></i></div>
                    <h3>Connectivity Matrix</h3>
                    <p>See all agent relationships in one high-density view. Spot problems at a glance.</p>
                </div>

                <div class="feature-card">
                    <div class="feature-orb orb-cyan"><i class="bi bi-bezier2"></i></div>
                    <h3>Live Topology</h3>
                    <p>Interactive D3 network maps. Path highlighting, shared hop detection - understand your network
                        visually.</p>
                </div>

                <div class="feature-card">
                    <div class="feature-orb orb-red"><i class="bi bi-bell"></i></div>
                    <h3>Smart Alerting</h3>
                    <p>Compound conditions, MTR path monitoring, resource alerts. No alert fatigue, just signal.</p>
                </div>

                <div class="feature-card">
                    <div class="feature-orb orb-pink"><i class="bi bi-building"></i></div>
                    <h3>Multi-Tenant</h3>
                    <p>Workspace isolation with member management. Perfect for MSPs who want to look like heroes.</p>
                </div>

                <div class="feature-card">
                    <div class="feature-orb orb-yellow"><i class="bi bi-database"></i></div>
                    <h3>ClickHouse Backend</h3>
                    <p>Horizontally scalable time-series storage. Millions of data points? No sweat.</p>
                </div>

                <div class="feature-card">
                    <div class="feature-orb orb-teal"><i class="bi bi-router"></i></div>
                    <h3>Network Insights</h3>
                    <p>Interface discovery, routing tables, MAC vendor lookup. Know your network inside and out.</p>
                </div>

                <div class="feature-card">
                    <div class="feature-orb orb-indigo"><i class="bi bi-share"></i></div>
                    <h3>Sharable Pages</h3>
                    <p>Time-limited public links with password protection. Share with vendors, ISPs, or clients
                        securely.</p>
                </div>
            </div>
        </div>
    </section>

    <!-- Comparison Section -->
    <section id="comparison" class="section section-dark">
        <div class="container">
            <div class="section-header">
                <span class="section-eyebrow">The Competition</span>
                <h2 class="section-title">Let's Talk Numbers</h2>
                <p class="section-subtitle">Because "contact sales" is just a fancy way of saying "we're expensive"
                </p>
            </div>

            <div class="comparison-wrapper">
                <div class="comparison-cards">
                    <div class="comp-card comp-them">
                        <div class="comp-header">
                            <span class="comp-name">Them</span>
                            <span class="comp-price">$300-2000<span>/mo</span></span>
                        </div>
                        <ul class="comp-features">
                            <li class="no"><i class="bi bi-x-lg"></i> Closed source black box</li>
                            <li class="no"><i class="bi bi-x-lg"></i> Per-agent pricing</li>
                            <li class="no"><i class="bi bi-x-lg"></i> Annual contracts</li>
                            <li class="no"><i class="bi bi-x-lg"></i> Sales calls required</li>
                            <li class="yes"><i class="bi bi-check-lg"></i> Decent monitoring</li>
                        </ul>
                    </div>

                    <div class="comp-card comp-us">
                        <div class="comp-badge">The Smart Choice</div>
                        <div class="comp-header">
                            <span class="comp-name">NetWatcher</span>
                            <span class="comp-price">$0<span>forever</span></span>
                        </div>
                        <ul class="comp-features">
                            <li class="yes"><i class="bi bi-check-lg"></i> 100% open source</li>
                            <li class="yes"><i class="bi bi-check-lg"></i> Unlimited agents (self-hosted)</li>
                            <li class="yes"><i class="bi bi-check-lg"></i> No contracts, ever</li>
                            <li class="yes"><i class="bi bi-check-lg"></i> Just sign up and go</li>
                            <li class="yes"><i class="bi bi-check-lg"></i> Better monitoring</li>
                        </ul>
                    </div>
                </div>

                <div class="comparison-table-wrapper">
                    <table class="comparison-table">
                        <thead>
                            <tr>
                                <th>Feature</th>
                                <th class="highlight">NetWatcher</th>
                                <th>SaaS Tools</th>
                                <th>Enterprise</th>
                            </tr>
                        </thead>
                        <tbody>
                            <tr class="category-row">
                                <td colspan="4">💰 Pricing Reality Check</td>
                            </tr>
                            <tr>
                                <td>Monthly Cost</td>
                                <td class="highlight"><strong class="price-tag">$0</strong></td>
                                <td>~$300 CAD (5 agents)</td>
                                <td>~$2,000+</td>
                            </tr>
                            <tr>
                                <td>Per-Agent Cost</td>
                                <td class="highlight"><strong class="price-tag free">FREE</strong></td>
                                <td>~$60 CAD/agent</td>
                                <td>Enterprise pricing</td>
                            </tr>
                            <tr>
                                <td>Self-Hosted Option</td>
                                <td class="highlight"><i class="bi bi-check-circle-fill text-success"></i></td>
                                <td><i class="bi bi-x-circle-fill text-danger"></i></td>
                                <td><i class="bi bi-x-circle-fill text-danger"></i></td>
                            </tr>
                            <tr class="category-row">
                                <td colspan="4">📊 Monitoring</td>
                            </tr>
                            <tr>
                                <td>Distributed Agents</td>
                                <td class="highlight"><i class="bi bi-check-circle-fill text-success"></i></td>
                                <td><i class="bi bi-check-circle-fill text-success"></i></td>
                                <td><i class="bi bi-check-circle-fill text-success"></i></td>
                            </tr>
                            <tr>
                                <td>MOS Score (VoIP Quality)</td>
                                <td class="highlight"><i class="bi bi-check-circle-fill text-success"></i></td>
                                <td><i class="bi bi-check-circle-fill text-success"></i></td>
                                <td><i class="bi bi-check-circle-fill text-success"></i></td>
                            </tr>
                            <tr>
                                <td>Connectivity Matrix</td>
                                <td class="highlight"><i class="bi bi-check-circle-fill text-success"></i></td>
                                <td><i class="bi bi-x-circle-fill text-danger"></i></td>
                                <td><i class="bi bi-check-circle-fill text-success"></i></td>
                            </tr>
                            <tr>
                                <td>SNMP Polling</td>
                                <td class="highlight"><span class="badge-roadmap">Coming Q2</span></td>
                                <td><i class="bi bi-check-circle-fill text-success"></i></td>
                                <td><i class="bi bi-check-circle-fill text-success"></i></td>
                            </tr>
                            <tr class="category-row">
                                <td colspan="4">🚨 Alerting</td>
                            </tr>
                            <tr>
                                <td>Compound Conditions (AND/OR)</td>
                                <td class="highlight"><i class="bi bi-check-circle-fill text-success"></i></td>
                                <td><i class="bi bi-x-circle-fill text-danger"></i></td>
                                <td><i class="bi bi-check-circle-fill text-success"></i></td>
                            </tr>
                            <tr>
                                <td>CPU/Memory Alerts</td>
                                <td class="highlight"><i class="bi bi-check-circle-fill text-success"></i></td>
                                <td><i class="bi bi-x-circle-fill text-danger"></i></td>
                                <td><i class="bi bi-check-circle-fill text-success"></i></td>
                            </tr>
                        </tbody>
                    </table>
                </div>
            </div>
        </div>
    </section>

    <!-- Roadmap Section -->
    <section id="roadmap" class="section">
        <div class="container">
            <div class="section-header">
                <span class="section-eyebrow">What's Next</span>
                <h2 class="section-title">Building in Public</h2>
                <p class="section-subtitle">Our roadmap is transparent because we have nothing to hide</p>
            </div>

            <div class="roadmap-timeline">
                <div class="roadmap-phase">
                    <div class="phase-header">
                        <span class="phase-label">Now</span>
                        <span class="phase-status live"><span class="live-dot"></span> Live</span>
                    </div>
                    <h3>Core Platform</h3>
                    <ul class="roadmap-items">
                        <li class="completed"><i class="bi bi-check-circle-fill"></i> Network Interface Selection</li>
                        <li class="completed"><i class="bi bi-check-circle-fill"></i> MOS Score (VoIP Quality)</li>
                        <li class="completed"><i class="bi bi-check-circle-fill"></i> Enhanced Alerting</li>
                        <li class="completed"><i class="bi bi-check-circle-fill"></i> Sharable Pages</li>
                        <li class="completed"><i class="bi bi-check-circle-fill"></i> DNS Probe</li>
                        <li class="in-progress"><i class="bi bi-arrow-repeat"></i> HTTP/HTTPS Probes</li>
                        <li class="in-progress"><i class="bi bi-arrow-repeat"></i> Dynamic Thresholds</li>
                        <li class="completed"><i class="bi bi-check-circle-fill"></i> Email Notifications</li>
                    </ul>
                </div>

                <div class="roadmap-phase">
                    <div class="phase-header">
                        <span class="phase-label">Q2 2026</span>
                        <span class="phase-status planned">Planned</span>
                    </div>
                    <h3>Probe Expansion</h3>
                    <ul class="roadmap-items">
                        <li class="highlight-item"><i class="bi bi-router"></i> SNMP Polling</li>
                        <li><i class="bi bi-signpost-2"></i> AS Path Resolution</li>
                    </ul>
                </div>

                <div class="roadmap-phase">
                    <div class="phase-header">
                        <span class="phase-label">Q3-Q4 2026</span>
                        <span class="phase-status planned">Planned</span>
                    </div>
                    <h3>Enterprise Ready</h3>
                    <ul class="roadmap-items">
                        <li><i class="bi bi-layout-wtf"></i> Custom Dashboards</li>
                        <li><i class="bi bi-file-earmark-pdf"></i> Scheduled Reports</li>
                        <li><i class="bi bi-key"></i> SSO (SAML/OIDC)</li>
                        <li><i class="bi bi-shield-lock"></i> RBAC Enhancements</li>
                    </ul>
                </div>
            </div>

            <div class="roadmap-cta">
                <a href="https://github.com/netwatcherio/oss/blob/master/docs/ROADMAP.md"
                    class="btn btn-outline btn-lg" target="_blank">
                    <i class="bi bi-kanban"></i> Full Roadmap on GitHub
                </a>
            </div>
        </div>
    </section>

    <!-- Pricing Section -->
    <section id="pricing" class="section section-dark">
        <div class="container">
            <div class="section-header">
                <span class="section-eyebrow">Pricing</span>
                <h2 class="section-title">Pick Your Flavor</h2>
                <p class="section-subtitle">Free, self-hosted, or managed - you always own your data</p>
            </div>

            <div class="pricing-grid">
                <div class="pricing-card" data-tilt>
                    <div class="pricing-visual">
                        <div class="visual-icon"><i class="bi bi-cloud"></i></div>
                    </div>
                    <div class="pricing-badge">Zero Friction</div>
                    <div class="pricing-header">
                        <h3>Cloud Free</h3>
                        <div class="pricing-price">
                            <span class="price-amount">$0</span>
                            <span class="price-period">forever</span>
                        </div>
                    </div>
                    <ul class="pricing-features">
                        <li><i class="bi bi-check-circle-fill"></i> Hosted on our infrastructure</li>
                        <li><i class="bi bi-check-circle-fill"></i> Up to 20 agents</li>
                        <li><i class="bi bi-check-circle-fill"></i> Up to 10 users</li>
                        <li><i class="bi bi-check-circle-fill"></i> 200 probes per agent</li>
                        <li><i class="bi bi-check-circle-fill"></i> Full feature access</li>
                    </ul>
                    <a href="https://app.netwatcher.io" class="btn btn-primary btn-block" target="_blank">
                        Start Free <i class="bi bi-arrow-right"></i>
                    </a>
                </div>

                <div class="pricing-card pricing-featured" data-tilt>
                    <div class="pricing-visual">
                        <div class="visual-icon visual-oss"><i class="bi bi-code-square"></i></div>
                    </div>
                    <div class="pricing-badge pricing-badge-alt">Full Control</div>
                    <div class="pricing-header">
                        <h3>Self-Hosted</h3>
                        <div class="pricing-price">
                            <span class="price-amount">$0</span>
                            <span class="price-period">forever</span>
                        </div>
                    </div>
                    <ul class="pricing-features">
                        <li><i class="bi bi-check-circle-fill"></i> Your infrastructure</li>
                        <li><i class="bi bi-check-circle-fill"></i> Unlimited agents</li>
                        <li><i class="bi bi-check-circle-fill"></i> Unlimited users</li>
                        <li><i class="bi bi-check-circle-fill"></i> Unlimited probes</li>
                        <li><i class="bi bi-check-circle-fill"></i> 100% data ownership</li>
                    </ul>
                    <router-link to="/#get-started" class="btn btn-outline btn-block">
                        Deploy Now <i class="bi bi-terminal"></i>
                    </router-link>
                </div>

                <div class="pricing-card" data-tilt>
                    <div class="pricing-visual">
                        <div class="visual-icon visual-pro"><i class="bi bi-stars"></i></div>
                    </div>
                    <div class="pricing-badge pricing-badge-pro">Hands Off</div>
                    <div class="pricing-header">
                        <h3>Managed</h3>
                        <div class="pricing-price">
                            <span class="price-amount">$149</span>
                            <span class="price-period">/month</span>
                        </div>
                    </div>
                    <ul class="pricing-features">
                        <li><i class="bi bi-check-circle-fill"></i> We run everything</li>
                        <li><i class="bi bi-check-circle-fill"></i> Unlimited everything</li>
                        <li><i class="bi bi-check-circle-fill"></i> Automatic updates</li>
                        <li><i class="bi bi-check-circle-fill"></i> Priority support</li>
                        <li><i class="bi bi-check-circle-fill"></i> Custom domain</li>
                    </ul>
                    <a href="mailto:hello@netwatcher.io?subject=Managed%20Cloud%20Inquiry"
                        class="btn btn-outline btn-block">
                        Get in Touch <i class="bi bi-envelope"></i>
                    </a>
                </div>
            </div>

            <div class="pricing-guarantee">
                <i class="bi bi-shield-check"></i>
                <p>Still 10x cheaper than the alternatives. Still 100% your data.</p>
            </div>
        </div>
    </section>

    <!-- Get Started Section -->
    <section id="get-started" class="section">
        <div class="container">
            <div class="section-header">
                <span class="section-eyebrow">Get Started</span>
                <h2 class="section-title">Start Monitoring in Minutes</h2>
                <p class="section-subtitle">Jump straight into our hosted platform or deploy on your own infrastructure</p>
            </div>

            <div class="get-started-grid">
                <!-- Primary: Cloud Sign-Up -->
                <div class="gs-card gs-card-primary">
                    <div class="gs-card-badge">Recommended</div>
                    <div class="gs-card-icon">
                        <i class="bi bi-cloud-check"></i>
                    </div>
                    <h3>Start Free in Seconds</h3>
                    <p class="gs-card-desc">Create a free account on our hosted platform — no install, no credit card, no Docker required. Just sign up and deploy agents.</p>
                    <ul class="gs-benefits">
                        <li><i class="bi bi-check-circle-fill"></i> Instant setup — monitoring in under 2 minutes</li>
                        <li><i class="bi bi-check-circle-fill"></i> Up to 20 agents &amp; 10 users free forever</li>
                        <li><i class="bi bi-check-circle-fill"></i> Automatic updates &amp; zero maintenance</li>
                        <li><i class="bi bi-check-circle-fill"></i> All features included, no paywalls</li>
                    </ul>
                    <a href="https://app.netwatcher.io" class="btn btn-primary btn-lg btn-block btn-glow" target="_blank">
                        <i class="bi bi-lightning-charge"></i> Create Free Account
                        <span class="btn-arrow">→</span>
                    </a>
                </div>

                <!-- Secondary: Self-Host -->
                <div class="gs-card gs-card-secondary">
                    <div class="gs-card-icon gs-card-icon-alt">
                        <i class="bi bi-terminal"></i>
                    </div>
                    <h3>Prefer Self-Hosting?</h3>
                    <p class="gs-card-desc">Full control, unlimited everything. Deploy with Docker in 60 seconds.</p>

                    <div class="terminal-window">
                        <div class="terminal-header">
                            <div class="terminal-buttons">
                                <span class="btn-red"></span>
                                <span class="btn-yellow"></span>
                                <span class="btn-green"></span>
                            </div>
                            <span class="terminal-title">netwatcher@oss:~</span>
                            <button class="copy-btn" @click="copyCode">
                                <i class="bi bi-clipboard"></i> Copy
                            </button>
                        </div>
                        <div class="terminal-body">
                            <pre><code><span class="comment"># 1. Clone the repo</span>
<span class="prompt">$</span> git clone https://github.com/netwatcherio/oss.git &amp;&amp; cd oss

<span class="comment"># 2. Configure</span>
<span class="prompt">$</span> cp .env.example .env &amp;&amp; nano .env

<span class="comment"># 3. Deploy</span>
<span class="prompt">$</span> docker-compose up -d</code></pre>
                        </div>
                    </div>

                    <div class="gs-secondary-links">
                        <a href="https://github.com/netwatcherio/oss" class="btn btn-outline btn-sm" target="_blank">
                            <i class="bi bi-github"></i> GitHub
                        </a>
                        <a href="https://github.com/netwatcherio/oss/blob/master/docs/README.md" class="btn btn-outline btn-sm" target="_blank">
                            <i class="bi bi-book"></i> Docs
                        </a>
                        <router-link to="/demo" class="btn btn-outline btn-sm">
                            <i class="bi bi-images"></i> Screenshots
                        </router-link>
                    </div>
                </div>
            </div>
        </div>
    </section>
</template>

<script setup>
import { ref, onMounted, nextTick } from 'vue'
import {
    initTypewriter,
    initNetworkAnimation,
    initCounters,
    initScrollAnimator,
    initTiltCards,
    initSmoothScroll,
} from '../composables/useAnimations.js'

const canvasRef = ref(null)
const typewriterRef = ref(null)

function copyCode() {
    const codeBlock = document.querySelector('.terminal-body code')
    const text = codeBlock.innerText

    navigator.clipboard.writeText(text).then(() => {
        const btn = document.querySelector('.copy-btn')
        const originalHTML = btn.innerHTML
        btn.innerHTML = '<i class="bi bi-check-lg"></i> Copied!'
        btn.style.color = '#22c55e'
        btn.style.borderColor = '#22c55e'

        setTimeout(() => {
            btn.innerHTML = originalHTML
            btn.style.color = ''
            btn.style.borderColor = ''
        }, 2000)
    })
}

onMounted(() => {
    nextTick(() => {
        if (typewriterRef.value) {
            initTypewriter(typewriterRef.value, '100% Open Source • AGPL-3.0', 80)
        }
        if (canvasRef.value) {
            initNetworkAnimation(canvasRef.value)
        }
        initCounters()
        initScrollAnimator()
        initTiltCards()
        initSmoothScroll()
    })
})
</script>
