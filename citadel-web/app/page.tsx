"use client";

import { useEffect, useRef } from "react";
import gsap from "gsap";
import { ScrollTrigger } from "gsap/ScrollTrigger";
import Link from "next/link";
import Image from "next/image";

gsap.registerPlugin(ScrollTrigger);

export default function Home() {
  const headerRef = useRef(null);
  const heroTitleRef = useRef(null);
  const heroSubtitleRef = useRef(null);
  const heroButtonRef = useRef(null);
  const feature1Ref = useRef(null);
  const feature2Ref = useRef(null);
  const architectureRef = useRef<HTMLDivElement | null>(null);
  const coreValuesRef = useRef(null);

  useEffect(() => {
    // Kill any existing ScrollTriggers to prevent issues on re-navigation
    ScrollTrigger.getAll().forEach(trigger => trigger.kill());

    // Smooth scroll
    gsap.to("html", {
      scrollBehavior: "smooth",
    });

    // Header fade in
    gsap.fromTo(headerRef.current,
      { opacity: 0, y: -20 },
      { opacity: 1, y: 0, duration: 0.8, ease: "power2.out" }
    );

    // Hero section sequential fade in
    gsap.fromTo(heroTitleRef.current,
      { opacity: 0, y: 30 },
      { opacity: 1, y: 0, duration: 1, delay: 0.3, ease: "power2.out" }
    );

    gsap.fromTo(heroSubtitleRef.current,
      { opacity: 0, y: 20 },
      { opacity: 1, y: 0, duration: 0.8, delay: 0.6, ease: "power2.out" }
    );

    gsap.fromTo(heroButtonRef.current,
      { opacity: 0, y: 20 },
      { opacity: 1, y: 0, duration: 0.8, delay: 0.9, ease: "power2.out" }
    );

    // Feature sections with scroll trigger
    gsap.fromTo(feature1Ref.current,
      { opacity: 0, x: -50 },
      {
        scrollTrigger: {
          trigger: feature1Ref.current,
          start: "top 80%",
          toggleActions: "play none none none",
        },
        opacity: 1,
        x: 0,
        duration: 1,
        ease: "power2.out",
      }
    );

    gsap.fromTo(feature2Ref.current,
      { opacity: 0, x: 50 },
      {
        scrollTrigger: {
          trigger: feature2Ref.current,
          start: "top 80%",
          toggleActions: "play none none none",
        },
        opacity: 1,
        x: 0,
        duration: 1,
        ease: "power2.out",
      }
    );

    // Architecture section fade in + pixel jump loop
    if (architectureRef.current) {
      gsap.fromTo(
        architectureRef.current,
        { opacity: 0, y: 40 },
        {
          scrollTrigger: {
            trigger: architectureRef.current,
            start: "top 80%",
            toggleActions: "play none none none",
          },
          opacity: 1,
          y: 0,
          duration: 1,
          ease: "power2.out",
        }
      );

      const items = architectureRef.current.querySelectorAll(".architecture-item");

      // Create individual timelines for each item with random offsets for out-of-sync animation
      const itemTimelines: gsap.core.Timeline[] = [];
      items.forEach((item, index) => {
        const tl = gsap.timeline({ repeat: -1, delay: index * 0.15 });
        // Subtle, frequent pixel jump animation
        tl.set(item, { y: 0 });
        tl.to(item, { y: -3, duration: 0, ease: "steps(1)" });
        tl.to({}, { duration: 0.5 });
        tl.to(item, { y: 0, duration: 0, ease: "steps(1)" });
        tl.to({}, { duration: 0.5 });
        itemTimelines.push(tl);
      });

      // Store timelines for cleanup
      (architectureRef.current as any)._itemTimelines = itemTimelines;

      const handlePointerDown = () => {
        itemTimelines.forEach(tl => tl.pause());
        gsap.to(items, { y: -3, duration: 0 });
        architectureRef.current?.classList.add("architecture-selected");
      };

      const handlePointerUp = () => {
        itemTimelines.forEach(tl => tl.play());
        architectureRef.current?.classList.remove("architecture-selected");
      };

      const wrapper = architectureRef.current.querySelector(".architecture-wrapper");
      if (wrapper) {
        wrapper.addEventListener("pointerdown", handlePointerDown);
        wrapper.addEventListener("pointerup", handlePointerUp);
        wrapper.addEventListener("pointerleave", handlePointerUp);
      }

      // Save cleanup for listeners
      (architectureRef.current as any)._architectureCleanup = () => {
        if (wrapper) {
          wrapper.removeEventListener("pointerdown", handlePointerDown);
          wrapper.removeEventListener("pointerup", handlePointerUp);
          wrapper.removeEventListener("pointerleave", handlePointerUp);
        }
      };
    }

    // Core Values fade in
    gsap.fromTo(coreValuesRef.current,
      { opacity: 0, y: 40 },
      {
        scrollTrigger: {
          trigger: coreValuesRef.current,
          start: "top 80%",
          toggleActions: "play none none none",
        },
        opacity: 1,
        y: 0,
        duration: 1,
        ease: "power2.out",
      }
    );

    // Cleanup on unmount
    return () => {
      ScrollTrigger.getAll().forEach(trigger => trigger.kill());
      if (architectureRef.current && (architectureRef.current as any)._itemTimelines) {
        (architectureRef.current as any)._itemTimelines.forEach((tl: gsap.core.Timeline) => tl.kill());
      }
      if (architectureRef.current && (architectureRef.current as any)._architectureCleanup) {
        (architectureRef.current as any)._architectureCleanup();
      }
    };
  }, []);

  return (
    <div className="bg-[#4A6C3A] flex flex-col items-center w-full min-h-screen overflow-x-hidden font-mono moving-lines">
      {/* Header */}
      <header ref={headerRef} className="relative w-full flex items-center justify-between px-[4vw] py-4 z-20">
        <div className="flex items-center gap-3">
          <div className="flex items-center gap-1">
            <span className="text-[#FFFEF1] text-2xl font-bold">[</span>
            <span className="text-[#FFFEF1] text-2xl font-bold">]</span>
          </div>
          <p className="font-bold text-[1.25rem] md:text-[1.5rem] text-[#FFFEF1] uppercase tracking-widest">
            CITADEL
          </p>
        </div>
        <nav className="hidden md:flex items-center gap-8">
          <Link href="/" className="font-bold text-[1rem] text-[#FFFEF1] uppercase tracking-widest hover:underline cursor-pointer">
            [HOME]
          </Link>
          <Link href="/faq" className="font-bold text-[1rem] text-[#FFFEF1] uppercase tracking-widest hover:underline cursor-pointer">
            [FAQS]
          </Link>
        </nav>
      </header>

      {/* Hero Section */}
      <section className="relative w-full flex flex-col items-center px-4 md:px-8 py-8 md:py-16">
        <div className="relative z-10 flex flex-col items-center text-center gap-6 md:gap-8 max-w-4xl">
          <div className="bg-[#FFFEF1] border-4 border-[#3D4A2C] p-8 md:p-12 shadow-[8px_8px_0px_0px_#3D4A2C]">
            <h1 ref={heroTitleRef} className="font-bold text-[1.5rem] md:text-[2rem] lg:text-[2.5rem] leading-tight text-[#3D4A2C] uppercase tracking-widest">
              <div>CONTROL YOUR DEVICES</div>
              <div>LIKE NEVER BEFORE.</div>
            </h1>
          </div>

          <p ref={heroSubtitleRef} className="text-[0.875rem] md:text-[1rem] text-[#FFFEF1] uppercase tracking-wider max-w-2xl leading-relaxed">
            <span className="block">Take back control of screen time and content with Citadel.</span>
            <span className="block mt-2">Works with any router. No cloud, no subscriptions.</span>
          </p>

          <Link
            href="/faq"
            ref={heroButtonRef}
            className="bg-[#3D4A2C] text-[#FFFEF1] px-8 py-4 border-4 border-[#FFFEF1] text-[0.875rem] md:text-[1rem] font-bold uppercase tracking-widest hover:bg-[#4A5A36] transition-colors shadow-[4px_4px_0px_0px_#FFFEF1]"
          >
            LEARN MORE →
          </Link>
        </div>
      </section>

      {/* Feature Section 1 */}
      <section ref={feature1Ref} className="relative w-full flex items-center justify-center py-8 md:py-12 px-4">
        <div className="w-full max-w-4xl">
          <div className="bg-[#FFFEF1] border-4 border-[#3D4A2C] p-6 md:p-10 shadow-[8px_8px_0px_0px_#3D4A2C]">
            <h2 className="font-bold text-[1.25rem] md:text-[1.75rem] lg:text-[2rem] leading-tight text-[#3D4A2C] uppercase tracking-widest text-center">
              <div>EVERYTHING IS ON</div>
              <div><span className="bg-[#3D4A2C] text-[#FFFEF1] px-2">YOUR</span> FINGERTIPS.</div>
            </h2>
            <div className="mt-6 flex justify-center gap-4">
              <span className="text-[#3D4A2C] text-sm uppercase tracking-wider">[VIEW]</span>
              <span className="text-[#3D4A2C] text-sm uppercase tracking-wider">[EDIT]</span>
              <span className="text-[#8B4513] text-sm uppercase tracking-wider">[DEL]</span>
            </div>
          </div>
        </div>
      </section>

      {/* Feature Section 2 */}
      <section ref={feature2Ref} className="relative w-full flex items-center justify-center py-8 md:py-12 px-4">
        <div className="w-full max-w-4xl">
          <div className="bg-[#FFFEF1] border-4 border-[#3D4A2C] p-6 md:p-10 shadow-[8px_8px_0px_0px_#3D4A2C]">
            <h2 className="font-bold text-[1.25rem] md:text-[1.75rem] lg:text-[2rem] leading-tight text-[#3D4A2C] uppercase tracking-widest text-center">
              SET LIMITATIONS.
            </h2>
            <div className="mt-6 grid grid-cols-3 gap-4 text-center text-[#3D4A2C] text-xs md:text-sm uppercase tracking-wider">
              <div className="border-2 border-[#3D4A2C] p-3">
                <div className="font-bold">DEVICE IDS</div>
              </div>
              <div className="border-2 border-[#3D4A2C] p-3">
                <div className="font-bold">USAGE (SEC)</div>
              </div>
              <div className="border-2 border-[#3D4A2C] p-3">
                <div className="font-bold">BUDGET (SEC)</div>
              </div>
            </div>
          </div>
        </div>
      </section>

      {/* Architecture Section */}
      <section
        ref={architectureRef}
        className="relative w-full flex flex-col items-center px-4 md:px-8 py-10 md:py-14"
      >
        <h2 className="font-bold text-[1.25rem] md:text-[1.75rem] text-[#FFFEF1] uppercase tracking-widest mb-6 md:mb-8 border-b-4 border-[#FFFEF1] pb-2">
          ARCHITECTURE
        </h2>

        <div
          className="architecture-wrapper relative w-full max-w-4xl bg-[#FFFEF1] border-4 border-[#3D4A2C] px-4 py-8 md:px-8 md:py-10 shadow-[8px_8px_0px_0px_#3D4A2C] cursor-pointer select-none"
        >
          <div className="architecture-inner relative">
            {/* Main layout using CSS Grid for precise positioning */}
            <div className="grid grid-cols-[1fr_auto_1fr] gap-0 items-start">
              {/* Left column: Curved Pipe above + Kid Computer below */}
              <div className="flex flex-col items-end justify-start">
                <div className="architecture-item">
                  <Image
                    src="/Curved-Pipe-PA.png"
                    alt="Curved Pipe"
                    width={140}
                    height={140}
                    className="pointer-events-none architecture-curved-pipe-left"
                  />
                </div>
                <div className="architecture-item kid_computer self-start flex flex-col items-center">
                  <Image
                    src="/Kid-Computer-PA.png"
                    alt="Child Computer"
                    width={120}
                    height={95}
                    className="pointer-events-none kid-computer"
                  />
                  <p className="text-[0.6rem] md:text-[0.7rem] text-[#3D4A2C] uppercase tracking-widest text-center mt-2 font-bold kid-computer-label">Child Device</p>
                </div>
              </div>

              {/* Center column: Router + Vertical Pipe + Parent Computer */}
              <div className="flex flex-col items-center">
                <p className="text-[0.6rem] md:text-[0.7rem] text-[#3D4A2C] uppercase tracking-widest text-center mb-1 font-bold">Switch</p>
                <div className="architecture-item architecture-item-main">
                  <Image
                    src="/Router-PA.png"
                    alt="Router"
                    width={180}
                    height={100}
                    className="pointer-events-none"
                  />
                </div>
                <div className="architecture-item">
                  <Image
                    src="/Straight-Pipe-PA.png"
                    alt="Vertical Pipe"
                    width={50}
                    height={120}
                    className="pointer-events-none architecture-vertical-pipe"
                  />
                </div>
                <div className="architecture-item">
                  <Image
                    src="/Parent-Computer-PA.png"
                    alt="Parent Computer"
                    width={110}
                    height={95}
                    className="pointer-events-none"
                  />
                  <p className="text-[0.6rem] md:text-[0.7rem] text-[#3D4A2C] uppercase tracking-widest text-center mt-1 font-bold">Parent Device</p>
                </div>
              </div>

              {/* Right column: Straight Pipe + Citadel Box */}
              <div className="flex items-start justify-start">
                <div className="architecture-item architecture-horizontal-pipe-container">
                  <Image
                    src="/Straight-Pipe-PA.png"
                    alt="Straight Pipe"
                    width={90}
                    height={20}
                    className="pointer-events-none architecture-horizontal-pipe"
                  />
                </div>
                <div className="architecture-item citadel-box">
                  <Image
                    src="/Citadel-Box-PA.png"
                    alt="Citadel Box"
                    width={100}
                    height={125}
                    className="pointer-events-none"
                  />
                </div>
              </div>
            </div>
          </div>
        </div>
      </section>

      {/* Core Values Section */}
      <section ref={coreValuesRef} className="relative w-full flex flex-col items-center px-4 md:px-8 py-8 md:py-12">
        <h2 className="font-bold text-[1.5rem] md:text-[2rem] text-[#FFFEF1] uppercase tracking-widest mb-6 md:mb-8 border-b-4 border-[#FFFEF1] pb-2">
          CORE VALUES
        </h2>

        <div className="w-full max-w-4xl bg-[#FFFEF1] border-4 border-[#3D4A2C] p-6 md:p-10 shadow-[8px_8px_0px_0px_#3D4A2C]">
          <p className="text-[0.875rem] md:text-[1rem] text-[#3D4A2C] uppercase tracking-wider leading-relaxed">
            The core goal of Citadel is to provide families with a simple, local, and reliable method to apply per-device and time-based internet rules. It resolves the issue of complex router settings and cloud privacy risks by acting as an affordable, hardware-agnostic DNS filtering appliance.
          </p>
        </div>
      </section>


      {/* Footer */}
      <footer className="w-full py-6 text-center">
        <p className="text-[#FFFEF1] text-xs uppercase tracking-widest">
          © 2025 CITADEL • ALL RIGHTS RESERVED
        </p>
      </footer>
    </div>
  );
}
