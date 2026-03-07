// Animations composable — ported from script.js
// Used by HomeView.vue for canvas animation, typewriter, counters, scroll effects, and tilt cards

// ========== Typewriter Effect ==========
export function initTypewriter(element, text, speed = 80) {
    let index = 0
    let isDeleting = false

    function type() {
        const current = text.substring(0, index)
        element.textContent = current

        let typeSpeed = speed

        if (isDeleting) {
            typeSpeed /= 2
        }

        if (!isDeleting && index === text.length) {
            typeSpeed = 3000
            isDeleting = true
        } else if (isDeleting && index === 0) {
            isDeleting = false
            typeSpeed = 500
        }

        if (isDeleting) {
            index--
        } else {
            index++
        }

        setTimeout(type, typeSpeed)
    }

    type()
}

// ========== Network Canvas Animation ==========
export function initNetworkAnimation(canvas) {
    const ctx = canvas.getContext('2d')
    let nodes = []
    let animationId = null

    function resize() {
        canvas.width = canvas.offsetWidth
        canvas.height = canvas.offsetHeight
    }

    function createNodes() {
        nodes = []
        const count = Math.floor((canvas.width * canvas.height) / 15000)
        for (let i = 0; i < count; i++) {
            nodes.push({
                x: Math.random() * canvas.width,
                y: Math.random() * canvas.height,
                vx: (Math.random() - 0.5) * 0.5,
                vy: (Math.random() - 0.5) * 0.5,
                radius: Math.random() * 2 + 1,
            })
        }
    }

    function draw() {
        ctx.clearRect(0, 0, canvas.width, canvas.height)

        // Draw connections
        for (let i = 0; i < nodes.length; i++) {
            for (let j = i + 1; j < nodes.length; j++) {
                const dx = nodes[i].x - nodes[j].x
                const dy = nodes[i].y - nodes[j].y
                const dist = Math.sqrt(dx * dx + dy * dy)

                if (dist < 150) {
                    ctx.beginPath()
                    ctx.strokeStyle = `rgba(59, 130, 246, ${0.1 * (1 - dist / 150)})`
                    ctx.lineWidth = 0.5
                    ctx.moveTo(nodes[i].x, nodes[i].y)
                    ctx.lineTo(nodes[j].x, nodes[j].y)
                    ctx.stroke()
                }
            }
        }

        // Draw and update nodes
        nodes.forEach((node) => {
            node.x += node.vx
            node.y += node.vy

            if (node.x < 0 || node.x > canvas.width) node.vx *= -1
            if (node.y < 0 || node.y > canvas.height) node.vy *= -1

            ctx.beginPath()
            ctx.fillStyle = 'rgba(59, 130, 246, 0.3)'
            ctx.arc(node.x, node.y, node.radius, 0, Math.PI * 2)
            ctx.fill()
        })

        animationId = requestAnimationFrame(draw)
    }

    resize()
    createNodes()
    draw()

    window.addEventListener('resize', () => {
        resize()
        createNodes()
    })
}

// ========== Counter Animation ==========
export function initCounters() {
    const counters = document.querySelectorAll('.ticker-value[data-target]')

    const observer = new IntersectionObserver(
        (entries) => {
            entries.forEach((entry) => {
                if (entry.isIntersecting) {
                    const el = entry.target
                    const target = parseInt(el.dataset.target)
                    const suffix = el.dataset.suffix || ''
                    const duration = 2000
                    const start = Date.now()

                    function update() {
                        const elapsed = Date.now() - start
                        const progress = Math.min(elapsed / duration, 1)
                        const eased = 1 - Math.pow(1 - progress, 3)
                        const current = Math.floor(eased * target)

                        if (target === 0) {
                            el.textContent = '$0'
                        } else {
                            el.textContent = current + suffix
                        }

                        if (progress < 1) {
                            requestAnimationFrame(update)
                        }
                    }

                    update()
                    observer.unobserve(el)
                }
            })
        },
        { threshold: 0.5 }
    )

    counters.forEach((counter) => observer.observe(counter))
}

// ========== Scroll Animations ==========
export function initScrollAnimator() {
    const observerOptions = {
        threshold: 0.1,
        rootMargin: '0px 0px -50px 0px',
    }

    const observer = new IntersectionObserver((entries) => {
        entries.forEach((entry) => {
            if (entry.isIntersecting) {
                entry.target.classList.add('animate-in')
            }
        })
    }, observerOptions)

    const elements = document.querySelectorAll(
        '.feature-card, .pricing-card, .pain-card, .comp-card, .roadmap-phase, .section-header'
    )

    elements.forEach((el, index) => {
        el.style.transitionDelay = `${(index % 4) * 0.08}s`
        observer.observe(el)
    })
}

// ========== Tilt Cards ==========
export function initTiltCards() {
    const tiltCards = document.querySelectorAll('[data-tilt]')

    tiltCards.forEach((card) => {
        card.addEventListener('mousemove', (e) => {
            const rect = card.getBoundingClientRect()
            const x = e.clientX - rect.left
            const y = e.clientY - rect.top
            const centerX = rect.width / 2
            const centerY = rect.height / 2

            const rotateX = ((y - centerY) / centerY) * -5
            const rotateY = ((x - centerX) / centerX) * 5

            card.style.transform = `perspective(1000px) rotateX(${rotateX}deg) rotateY(${rotateY}deg) scale(1.02)`
        })

        card.addEventListener('mouseleave', () => {
            card.style.transform = 'perspective(1000px) rotateX(0) rotateY(0) scale(1)'
        })
    })
}

// ========== Smooth Scroll for Hash Links ==========
export function initSmoothScroll() {
    document.addEventListener('click', (e) => {
        const link = e.target.closest('a[href^="#"]')
        if (link) {
            e.preventDefault()
            const targetId = link.getAttribute('href').slice(1)
            const targetElement = document.getElementById(targetId)
            if (targetElement) {
                targetElement.scrollIntoView({ behavior: 'smooth' })
            }
        }
    })
}
