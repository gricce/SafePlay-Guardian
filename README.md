# Project Charter: SafePlay Guardian

**Version:** 0.1 (Initial Draft – May 2026)
**Author:** Gabriel Ricce / gabriel.ricce@gmail.com

## Mission

Build a privacy-first, self-hosted, open-source parental control platform specialized in protecting children from online grooming and abuse in gaming environments (including mobile games like Animal Planet titles, Roblox, Minecraft, and immersive VR platforms like Meta Quest). The project empowers parents with transparent, explainable tools while addressing sophisticated threats like age bypasses, device hijacking, ephemeral voice/video chats, nickname hopping, and VR-based sexual simulation.

## Vision

Create an auditable, community-driven alternative to closed-source commercial tools. Emphasize local-first processing, ethical design, and defense-in-depth to fill gaps in existing solutions.

## Problem Statement & Threat Model

Children face advanced grooming in kid-oriented and general gaming platforms. Key threats include:

- **Age & Account Bypass:** Self-reported age gates are trivial to circumvent.
- **Device Control Bypass:** Password hijacking, VPN/proxies, Screen Time exploits on iOS/Android.
- **Ephemeral Communication:** Voice/video chats with no persistent logs; quick tunneling to external apps (Discord, Snapchat).
- **Adaptive Predator Tactics:** Disposable nicknames, code verification phrases, targeted re-approaches.
- **VR Amplification (Meta Quest etc.):** Avatar-based sexual simulation, full-body tracking, immersive private rooms, higher harassment rates.
- **Platform Limitations:** Weak moderation, profit-driven growth over safety.

**Goal:** Provide layered protection that works even when kids are motivated and tech-savvy.

## Core Principles

- **Privacy-First:** Local ML processing where possible, data minimization, user-controlled encryption.
- **Transparency & Consent:** Visible indicators on child devices; explicit parental consent; no hidden "stealth" modes.
- **Explainability:** Every alert shows clear reasoning.
- **Ethical & Legal Compliance:** Designed for responsible parents only; strong disclaimers.
- **Open & Extensible:** Modular for community contributions (new detectors, game integrations).
- **Resilience:** Tamper detection and fallback layers.

## Target Users

- Parents/guardians of children (primarily under 16).
- Technically inclined families who value self-hosting and auditability.

## Key Features

### MVP (Phase 1)

- Screen time, app limits, and usage reporting.
- Basic device monitoring (Android stronger; iOS via available APIs).
- On-device ML for text/chat grooming pattern detection.
- Metadata monitoring for voice sessions (duration, new participants).
- Parent dashboard (web/self-hosted).
- Education module for kids and parents.
- Selective screen mirroring (escalation only, with visible indicator and consent).

### Phase 2 (Advanced)

- Local speech-to-text + anomaly detection for voice (adult voice, secrecy language).
- VR session detection and monitoring (Quest integration via available tools).
- Selective short-term recording (voice/video) on high-confidence alerts, with auto-delete.
- Network/router-level monitoring (VPN detection, risky domains).
- Behavioral analysis (friend graph changes, off-hours activity, nickname patterns).
- Anonymized community threat intel sharing (opt-in, privacy-preserving).

### Non-Goals

- Replacement for law enforcement or professional therapy.
- Always-on hidden surveillance.
- Cloud-only dependency (self-hosted preferred).
- Support for unauthorized monitoring.

## High-Level Architecture

1. **Client Layer** — Agent on child device (Android: Accessibility + MediaProjection; iOS: limited APIs + Screen Time).
2. **Processing Layer** — On-device ML (TensorFlow Lite / ONNX / Whisper.cpp) + optional self-hosted backend (Docker).
3. **Escalation Layer** — Parent-approved mirroring/recording with transparency.
4. **Parent Interface** — React-based dashboard.
5. **Security** — Least privilege, tamper resistance, audit logs.

### Repo Structure

```
/cmd/platform/          main.go — wires everything, starts service
/internal/
    device/             registry: models + storage
    policy/             rules, schedules, blocklists
    enforce/            Enforcer interface
        agent/          agent backend
        network/        DNS/proxy backends (later)
    event/              logging + reporting queries
    api/                HTTP handlers, mounts dashboard
/web/                   SPA source, built into /web/dist
/agent/                 separate module — the on-device agent (Go or Rust)
```

### Stack

- **Parents PC:** Go (compiled)
- **Mobile/Kids:** Flutter

## Differentiation from Existing Projects

Existing open-source tools (Child Screen Time, KidSafe, KidShield, Little Brother, Kidlogger) provide good foundations for screen time and basic monitoring but lack:

- Deep gaming/VR focus.
- Advanced voice/video anomaly detection.
- Strong bypass resistance and explainable AI.
- Comprehensive threat model for grooming + nickname hopping.

This project will extend and integrate where possible while filling these gaps.

## Legal & Ethical Guardrails

- Require verifiable parental consent.
- Comply with COPPA, GDPR, CCPA/CPRA, and two-party consent (CA).
- Strong disclaimers against misuse.
- Report real abuse to authorities (NCMEC CyberTipline).

## Organizations for References & Collaboration

Here are key organizations working in child online safety. You can reference them in the README and reach out for guidance, datasets (anonymized), or partnerships:

- **National Center for Missing & Exploited Children (NCMEC)** — CyberTipline, NetSmartz resources. Primary reporting hub.
- **Thorn** — Tech-focused, builds tools against CSAM/grooming, strong research on gaming.
- **Internet Watch Foundation (IWF)** — UK-based, content removal and research.
- **Family Online Safety Institute (FOSI)** — Policy, best practices, Good Digital Parenting resources.
- **ESRB (Entertainment Software Rating Board)** — Gaming-specific parental control guides.
- **Internet Matters** — Practical parent resources, UK-focused but useful globally.
- **Common Sense Media** — Reviews and education for games/apps.
- **WePROTECT Global Alliance** — International coordination on online child protection.

**Additional:** INHOPE, UNICEF child online safety initiatives.

## Risks & Mitigations

| Risk | Mitigation |
|------|------------|
| Legal liability | Strong disclaimers + legal review. |
| Misuse | Consent enforcement + Code of Conduct. |
| Technical bypass | Layered defense + community updates. |
| Low adoption | Excellent documentation and early parent feedback. |
