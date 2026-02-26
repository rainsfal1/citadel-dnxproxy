# Citadel Web Interface

This is the Next.js frontend for **Citadel**, providing a modern, dynamic web interface to control devices and internet access policies.

## Getting Started

First, install dependencies:

```bash
npm install
```

Then, run the development server:

```bash
npm run dev
# or
yarn dev
# or
pnpm dev
# or
bun dev
```

Open [http://localhost:3000](http://localhost:3000) with your browser to see the result.

## Architecture & Integration

The web interface connects to the Citadel DNS Proxy controller to securely manage user constraints, device access, and time-window budgets.

- **Frontend:** Next.js (React), Tailwind CSS, GSAP for animations.
- **Backend Communication:** Interacts via HTTP API to update the SQLite-backed policy store.
