<template>
    <!-- Hero -->
    <header class="demo-hero">
        <div class="container">
            <span class="section-eyebrow">Visual Proof</span>
            <h1>See It <span class="gradient-text">In Action</span></h1>
            <p class="hero-subtitle">
                Screenshots that prove we actually built this thing.
                <br>No mockups, no Figma fantasies — just real code doing real monitoring.
            </p>
        </div>
    </header>

    <!-- Demo Content -->
    <section class="section" style="padding-top: 40px;">
        <div class="container">

            <!-- Video Demo -->
            <div class="demo-grid">
                <div class="demo-item">
                    <div class="demo-placeholder video-placeholder">
                        <i class="bi bi-play-circle-fill"></i>
                        <span>Demo Video Coming Soon</span>
                    </div>
                    <div class="demo-content">
                        <h3>5-Minute Quick Start <span class="coming-soon-badge">Coming Soon</span></h3>
                        <p>Watch us deploy NetWatcher from zero to monitoring in 5 minutes flat. No cuts, no tricks,
                            just Docker doing its thing.</p>
                        <div class="demo-tags">
                            <span class="demo-tag">Video</span>
                            <span class="demo-tag">Docker</span>
                            <span class="demo-tag">Walkthrough</span>
                        </div>
                    </div>
                </div>
            </div>

            <div class="section-header" style="margin-bottom: 48px;">
                <span class="section-eyebrow">Screenshots</span>
                <h2 class="section-title">The Interface</h2>
                <p class="section-subtitle">What you'll actually see when you log in</p>
            </div>

            <div class="demo-gallery">
                <div class="demo-item" v-for="item in screenshots" :key="item.img">
                    <img :src="item.img" :alt="item.title" @click="openLightbox(item)">
                    <div class="demo-content">
                        <h3>{{ item.title }}</h3>
                        <p>{{ item.description }}</p>
                        <div class="demo-tags">
                            <span class="demo-tag" v-for="tag in item.tags" :key="tag">{{ tag }}</span>
                        </div>
                    </div>
                </div>
            </div>
        </div>
    </section>

    <!-- CTA -->
    <section class="demo-cta">
        <div class="container">
            <h2>Convinced Yet?</h2>
            <p>Deploy it yourself in less time than it takes to fill out an enterprise quote form.</p>
            <div style="display: flex; justify-content: center; gap: 16px; flex-wrap: wrap;">
                <router-link to="/#get-started" class="btn btn-primary btn-lg btn-glow">
                    <i class="bi bi-rocket-takeoff"></i> Deploy Now
                </router-link>
                <a href="https://github.com/netwatcherio/oss" class="btn btn-outline btn-lg" target="_blank">
                    <i class="bi bi-github"></i> View Source
                </a>
            </div>
        </div>
    </section>

    <!-- Lightbox Modal -->
    <div class="lightbox" :class="{ active: lightboxActive }" @click="closeLightbox($event)">
        <div class="lightbox-content">
            <button class="lightbox-close" @click="closeLightbox($event)">&times;</button>
            <img :src="lightboxSrc" :alt="lightboxCaption">
            <div class="lightbox-caption">{{ lightboxCaption }}</div>
        </div>
    </div>
</template>

<script setup>
import { ref, onMounted, onUnmounted, nextTick } from 'vue'

const screenshots = [
    {
        img: '/assets/img_1.png',
        title: 'Workspaces Overview',
        description: 'Multi-tenant workspace management with agent counts, member access, and quick workspace switching. Keep your clients separate without the enterprise price tag.',
        tags: ['Dashboard', 'Multi-tenant'],
    },
    {
        img: '/assets/img_2.png',
        title: 'Network Topology Map',
        description: 'Interactive D3 visualization showing agent relationships, network paths, and real-time connectivity. Actually understand your network topology for once.',
        tags: ['Topology', 'D3.js'],
    },
    {
        img: '/assets/img_3.png',
        title: 'Destination Overview',
        description: "Comprehensive probe listing with targets, endpoints, latency, loss, and agent assignments. Everything you need, nothing you don't.",
        tags: ['Probes', 'Overview'],
    },
    {
        img: '/assets/img_4.png',
        title: 'Connectivity Matrix',
        description: 'High-density mesh view showing all agent-to-target relationships. Color-coded health status at a glance. Pattern recognition for humans.',
        tags: ['Visualization', 'Matrix'],
    },
    {
        img: '/assets/img_5.png',
        title: 'Latency Metrics Dashboard',
        description: 'Real-time latency, jitter, and packet loss charts with statistical summaries. Historical trends that actually help you find problems.',
        tags: ['Metrics', 'Charts'],
    },
    {
        img: '/assets/img_6.png',
        title: 'Voice Quality Score (MOS)',
        description: 'Aggregated voice quality scoring from multiple data sources. Because your VoIP system deserves monitoring too.',
        tags: ['MOS', 'VoIP'],
    },
    {
        img: '/assets/img_7.png',
        title: 'Traceroute Topology',
        description: 'Interactive network path visualization with shared hop detection and status indicators. See exactly where your packets wander.',
        tags: ['Traceroute', 'Path Analysis'],
    },
    {
        img: '/assets/img_8.png',
        title: 'Network Path Details',
        description: 'Hop-by-hop details with hostname resolution, IP addresses, latency metrics, and packet loss analysis. Deep visibility, zero mystery.',
        tags: ['Topology', 'Details'],
    },
    {
        img: '/assets/img_9.png',
        title: 'MTR Trace History',
        description: "Historical traceroute data with hop counts, latency trends, and quick access to view all traces. Your network's memory.",
        tags: ['MTR', 'History'],
    },
    {
        img: '/assets/img_10.png',
        title: 'Route Change Detection',
        description: 'Side-by-side comparison of previous and current routes. Automatic detection and highlighting of path changes. Know when your ISP "optimizes" things.',
        tags: ['Route Changes', 'Comparison'],
    },
]

const lightboxActive = ref(false)
const lightboxSrc = ref('')
const lightboxCaption = ref('')

function openLightbox(item) {
    lightboxSrc.value = item.img
    lightboxCaption.value = item.title
    lightboxActive.value = true
    document.body.style.overflow = 'hidden'
}

function closeLightbox(event) {
    if (event.target.classList.contains('lightbox') || event.target.classList.contains('lightbox-close')) {
        lightboxActive.value = false
        document.body.style.overflow = ''
    }
}

function handleEscape(e) {
    if (e.key === 'Escape' && lightboxActive.value) {
        lightboxActive.value = false
        document.body.style.overflow = ''
    }
}

onMounted(() => {
    document.addEventListener('keydown', handleEscape)

    nextTick(() => {
        // Scroll animations for demo items
        const observerOptions = {
            threshold: 0.1,
            rootMargin: '0px 0px -50px 0px',
        }

        const observer = new IntersectionObserver((entries) => {
            entries.forEach((entry) => {
                if (entry.isIntersecting) {
                    entry.target.style.opacity = '1'
                    entry.target.style.transform = 'translateY(0)'
                }
            })
        }, observerOptions)

        document.querySelectorAll('.demo-item').forEach((el, index) => {
            el.style.opacity = '0'
            el.style.transform = 'translateY(30px)'
            el.style.transition = `opacity 0.5s ease ${index * 0.05}s, transform 0.5s ease ${index * 0.05}s`
            observer.observe(el)
        })
    })
})

onUnmounted(() => {
    document.removeEventListener('keydown', handleEscape)
    document.body.style.overflow = ''
})
</script>

<style>
.demo-hero {
    position: relative;
    padding: 180px 0 80px;
    text-align: center;
    overflow: hidden;
}

.demo-hero::before {
    content: '';
    position: absolute;
    top: 0;
    left: 0;
    right: 0;
    bottom: 0;
    background: radial-gradient(ellipse at center, rgba(59, 130, 246, 0.08) 0%, transparent 70%);
    pointer-events: none;
}

.demo-hero .container {
    position: relative;
    z-index: 1;
}

.demo-hero h1 {
    font-size: clamp(36px, 6vw, 56px);
    font-weight: 800;
    margin-bottom: 16px;
}

.demo-grid {
    display: grid;
    gap: 32px;
    margin-bottom: 60px;
}

.demo-item {
    background: var(--bg-card);
    border: 1px solid var(--border-color);
    border-radius: 20px;
    overflow: hidden;
    transition: all 0.4s cubic-bezier(0.4, 0, 0.2, 1);
    position: relative;
}

.demo-item::before {
    content: '';
    position: absolute;
    top: 0;
    left: 0;
    right: 0;
    height: 3px;
    background: var(--accent-gradient);
    transform: scaleX(0);
    transition: transform 0.4s;
}

.demo-item:hover {
    transform: translateY(-8px);
    border-color: var(--border-color-light);
    box-shadow: 0 24px 60px rgba(0, 0, 0, 0.3);
}

.demo-item:hover::before {
    transform: scaleX(1);
}

.demo-placeholder {
    background: linear-gradient(135deg, var(--bg-dark) 0%, var(--bg-dark-secondary) 100%);
    aspect-ratio: 16/9;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    color: var(--text-muted);
    border-bottom: 1px solid var(--border-color);
    position: relative;
    overflow: hidden;
}

.demo-placeholder::after {
    content: '';
    position: absolute;
    width: 200px;
    height: 200px;
    background: var(--accent-primary);
    border-radius: 50%;
    filter: blur(80px);
    opacity: 0.15;
}

.demo-placeholder i {
    font-size: 64px;
    margin-bottom: 16px;
    opacity: 0.6;
    position: relative;
    z-index: 1;
}

.demo-placeholder span {
    font-size: 14px;
    position: relative;
    z-index: 1;
}

.demo-content {
    padding: 28px;
}

.demo-content h3 {
    font-size: 20px;
    font-weight: 600;
    margin-bottom: 10px;
    display: flex;
    align-items: center;
    gap: 12px;
}

.demo-content p {
    font-size: 14px;
    color: var(--text-secondary);
    margin-bottom: 16px;
    line-height: 1.6;
}

.demo-tags {
    display: flex;
    gap: 8px;
    flex-wrap: wrap;
}

.demo-tag {
    background: rgba(59, 130, 246, 0.1);
    color: var(--accent-primary);
    font-size: 12px;
    font-weight: 500;
    padding: 5px 12px;
    border-radius: 6px;
    transition: all 0.2s;
}

.demo-tag:hover {
    background: rgba(59, 130, 246, 0.2);
}

.video-placeholder {
    background: linear-gradient(135deg, #111827 0%, #0d1219 100%);
}

.video-placeholder i {
    font-size: 72px;
    background: var(--accent-gradient);
    -webkit-background-clip: text;
    -webkit-text-fill-color: transparent;
    background-clip: text;
    opacity: 0.8;
    animation: pulse-icon 2s ease infinite;
}

@keyframes pulse-icon {

    0%,
    100% {
        transform: scale(1);
        opacity: 0.8;
    }

    50% {
        transform: scale(1.1);
        opacity: 1;
    }
}

.coming-soon-badge {
    background: rgba(245, 158, 11, 0.15);
    color: var(--warning);
    font-size: 11px;
    font-weight: 600;
    padding: 4px 10px;
    border-radius: 6px;
    text-transform: uppercase;
    letter-spacing: 0.5px;
}

.demo-cta {
    text-align: center;
    padding: 100px 0;
    background: var(--bg-dark-secondary);
    border-top: 1px solid var(--border-color);
    position: relative;
    overflow: hidden;
}

.demo-cta::before {
    content: '';
    position: absolute;
    top: 50%;
    left: 50%;
    transform: translate(-50%, -50%);
    width: 600px;
    height: 600px;
    background: radial-gradient(circle, rgba(59, 130, 246, 0.1) 0%, transparent 70%);
    pointer-events: none;
}

.demo-cta .container {
    position: relative;
    z-index: 1;
}

.demo-cta h2 {
    font-size: 36px;
    font-weight: 800;
    margin-bottom: 12px;
}

.demo-cta p {
    color: var(--text-secondary);
    margin-bottom: 32px;
    font-size: 18px;
}

/* Lightbox Modal */
.lightbox {
    display: none;
    position: fixed;
    top: 0;
    left: 0;
    width: 100%;
    height: 100%;
    background: rgba(0, 0, 0, 0.95);
    z-index: 10000;
    align-items: center;
    justify-content: center;
    padding: 20px;
    box-sizing: border-box;
    cursor: pointer;
    opacity: 0;
    transition: opacity 0.3s ease;
    backdrop-filter: blur(10px);
}

.lightbox.active {
    display: flex;
    opacity: 1;
}

.lightbox-content {
    position: relative;
    max-width: 95%;
    max-height: 95%;
    cursor: default;
}

.lightbox-content img {
    max-width: 100%;
    max-height: 90vh;
    border-radius: 12px;
    box-shadow: 0 24px 80px rgba(0, 0, 0, 0.5);
    transform: scale(0.9);
    transition: transform 0.3s ease;
}

.lightbox.active .lightbox-content img {
    transform: scale(1);
}

.lightbox-close {
    position: absolute;
    top: -50px;
    right: 0;
    background: rgba(255, 255, 255, 0.1);
    border: 1px solid rgba(255, 255, 255, 0.2);
    color: white;
    width: 44px;
    height: 44px;
    border-radius: 50%;
    font-size: 24px;
    cursor: pointer;
    display: flex;
    align-items: center;
    justify-content: center;
    transition: all 0.2s;
}

.lightbox-close:hover {
    background: rgba(255, 255, 255, 0.2);
    transform: scale(1.1) rotate(90deg);
}

.lightbox-caption {
    position: absolute;
    bottom: -50px;
    left: 0;
    right: 0;
    text-align: center;
    color: white;
    font-size: 16px;
    font-weight: 500;
}

.demo-item img {
    cursor: zoom-in;
    transition: all 0.3s;
    width: 100%;
    aspect-ratio: 16/9;
    object-fit: cover;
    border-bottom: 1px solid var(--border-color);
}

.demo-item img:hover {
    opacity: 0.9;
}

.demo-gallery {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(400px, 1fr));
    gap: 32px;
}

@media (max-width: 768px) {
    .demo-gallery {
        grid-template-columns: 1fr;
    }

    .demo-hero {
        padding: 140px 0 60px;
    }
}
</style>
