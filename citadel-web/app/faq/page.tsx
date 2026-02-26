"use client";

import { useEffect, useRef, useState } from "react";
import gsap from "gsap";
import { ScrollTrigger } from "gsap/ScrollTrigger";
import Link from "next/link";

gsap.registerPlugin(ScrollTrigger);

interface FAQItem {
  question: string;
  answer: string;
}

const faqs: FAQItem[] = [
  {
    question: "WHAT IS CITADEL?",
    answer:
      "Citadel is a compact network appliance built around a RISC-V system on chip. It sits inside your home network and acts as the primary DNS server for all child devices, filtering content and enforcing screen time rules locally.",
  },
  {
    question: "HOW DOES CITADEL WORK?",
    answer:
      "Every DNS query from child devices passes through Citadel's embedded policy engine. It evaluates requests using device-specific rules and time windows. Allowed queries are forwarded to upstream DNS servers while restricted queries are blocked locally.",
  },
  {
    question: "DO I NEED TO REPLACE MY ROUTER?",
    answer:
      "No. Citadel is hardware-agnostic and works with any existing router. Simply configure child devices to use Citadel as their DNS server. No expensive hardware upgrades required.",
  },
  {
    question: "DOES CITADEL REQUIRE INTERNET OR CLOUD SERVICES?",
    answer:
      "No. Citadel operates entirely locally on your home network. All policy evaluation happens on the device itself, ensuring privacy and reliability without depending on external cloud services.",
  },
  {
    question: "WHAT CAN I CONTROL WITH CITADEL?",
    answer:
      "You can set per-device time windows (e.g., 'Ali's tablet only has internet from 4pm to 8pm'), block specific domains or categories (social media, adult content), and maintain separate profiles for each child.",
  },
  {
    question: "CAN I SEE WHAT MY CHILDREN ARE ACCESSING?",
    answer:
      "Yes. Citadel logs every DNS decision with timestamp, domain, device, and action taken. Parents can review these logs through the management interface.",
  },
  {
    question: "WHAT TECHNOLOGY DOES CITADEL USE?",
    answer:
      "Citadel runs on a RISC-V 64-bit Linux environment. The DNS Policy Engine is written in Go and cross-compiled for RISC-V architecture. Configuration is stored in JSON format.",
  },
  {
    question: "IS CITADEL DIFFICULT TO SET UP?",
    answer:
      "No. Citadel is designed for non-technical households. Parents manage profiles through a simple configuration interface without needing advanced technical knowledge.",
  },
  {
    question: "WHO BUILT CITADEL?",
    answer:
      "Citadel is a Computer Architecture project developed by Zyan Ahmed, Khizer Inam, and Mohammad Abdullah from BSCS 13-A.",
  },
];

function FAQCard({
  item,
  isOpen,
  onToggle,
}: {
  item: FAQItem;
  isOpen: boolean;
  onToggle: () => void;
}) {
  return (
    <div className="w-full">
      <div
        className={`bg-[#FFFEF1] border-4 border-[#3D4A2C] shadow-[6px_6px_0px_0px_#3D4A2C] cursor-pointer transition-all hover:translate-x-1 hover:translate-y-1 hover:shadow-[4px_4px_0px_0px_#3D4A2C] ${
          isOpen ? "mb-0" : ""
        }`}
        onClick={onToggle}
      >
        <div className="flex items-center justify-between p-4 md:p-6">
          <h3 className="font-bold text-[0.75rem] md:text-[1rem] text-[#3D4A2C] uppercase tracking-widest pr-4">
            {item.question}
          </h3>
          <span className="text-[#3D4A2C] font-bold text-xl flex-shrink-0">
            {isOpen ? "[-]" : "[+]"}
          </span>
        </div>
        {isOpen && (
          <div className="border-t-4 border-[#3D4A2C] p-4 md:p-6 bg-[#E8E0C8]">
            <p className="text-[0.75rem] md:text-[0.875rem] text-[#3D4A2C] uppercase tracking-wider leading-relaxed">
              {item.answer}
            </p>
          </div>
        )}
      </div>
    </div>
  );
}

export default function FAQPage() {
  const headerRef = useRef(null);
  const titleRef = useRef(null);
  const subtitleRef = useRef(null);
  const faqContainerRef = useRef(null);
  const footerRef = useRef(null);
  const [openIndex, setOpenIndex] = useState<number | null>(0);

  useEffect(() => {
    // Kill any existing ScrollTriggers to prevent issues on re-navigation
    ScrollTrigger.getAll().forEach(trigger => trigger.kill());

    // Header fade in
    gsap.fromTo(headerRef.current,
      { opacity: 0, y: -20 },
      { opacity: 1, y: 0, duration: 0.8, ease: "power2.out" }
    );

    // Title fade in
    gsap.fromTo(titleRef.current,
      { opacity: 0, y: 30 },
      { opacity: 1, y: 0, duration: 1, delay: 0.3, ease: "power2.out" }
    );

    // Subtitle fade in
    gsap.fromTo(subtitleRef.current,
      { opacity: 0, y: 20 },
      { opacity: 1, y: 0, duration: 0.8, delay: 0.5, ease: "power2.out" }
    );

    // FAQ cards staggered fade in
    gsap.fromTo(faqContainerRef.current,
      { opacity: 0, y: 40 },
      { opacity: 1, y: 0, duration: 1, delay: 0.7, ease: "power2.out" }
    );

    // Footer fade in with scroll trigger
    gsap.fromTo(footerRef.current,
      { opacity: 0 },
      {
        scrollTrigger: {
          trigger: footerRef.current,
          start: "top 95%",
          toggleActions: "play none none none",
        },
        opacity: 1,
        duration: 0.8,
        ease: "power2.out",
      }
    );

    // Cleanup on unmount
    return () => {
      ScrollTrigger.getAll().forEach(trigger => trigger.kill());
    };
  }, []);

  const handleToggle = (index: number) => {
    setOpenIndex(openIndex === index ? null : index);
  };

  return (
    <div className="bg-[#4A6C3A] flex flex-col items-center w-full min-h-screen overflow-x-hidden font-mono moving-lines">
      {/* Header */}
      <header
        ref={headerRef}
        className="relative w-full flex items-center justify-between px-[4vw] py-4 z-20"
      >
        <Link href="/" className="flex items-center gap-3">
          <div className="flex items-center gap-1">
            <span className="text-[#FFFEF1] text-2xl font-bold">[</span>
            <span className="text-[#FFFEF1] text-2xl font-bold">]</span>
          </div>
          <p className="font-bold text-[1.25rem] md:text-[1.5rem] text-[#FFFEF1] uppercase tracking-widest">
            CITADEL
          </p>
        </Link>
        <nav className="hidden md:flex items-center gap-8">
          <Link
            href="/"
            className="font-bold text-[1rem] text-[#FFFEF1] uppercase tracking-widest hover:underline cursor-pointer"
          >
            [HOME]
          </Link>
          <Link
            href="/faq"
            className="font-bold text-[1rem] text-[#FFFEF1] uppercase tracking-widest hover:underline cursor-pointer"
          >
            [FAQS]
          </Link>
        </nav>
      </header>

      {/* Title Section */}
      <section className="relative z-10 w-full flex flex-col items-center px-4 md:px-8 py-8 md:py-12">
        <div
          ref={titleRef}
          className="bg-[#FFFEF1] border-4 border-[#3D4A2C] p-6 md:p-10 shadow-[8px_8px_0px_0px_#3D4A2C] mb-8"
        >
          <h1 className="font-bold text-[1.5rem] md:text-[2rem] lg:text-[2.5rem] leading-tight text-[#3D4A2C] uppercase tracking-widest text-center">
            FREQUENTLY ASKED QUESTIONS
          </h1>
        </div>

        <p ref={subtitleRef} className="text-[0.875rem] md:text-[1rem] text-[#FFFEF1] uppercase tracking-wider max-w-2xl text-center leading-relaxed mb-8">
          EVERYTHING YOU NEED TO KNOW ABOUT CITADEL
        </p>
      </section>

      {/* FAQ Section */}
      <section className="relative z-10 w-full flex flex-col items-center px-4 md:px-8 pb-12">
        <div ref={faqContainerRef} className="w-full max-w-3xl flex flex-col gap-4">
          {faqs.map((faq, index) => (
            <FAQCard
              key={index}
              item={faq}
              isOpen={openIndex === index}
              onToggle={() => handleToggle(index)}
            />
          ))}
        </div>
      </section>

      {/* Footer */}
      <footer ref={footerRef} className="relative z-10 w-full py-6 text-center">
        <p className="text-[#FFFEF1] text-xs uppercase tracking-widest">
          © 2025 CITADEL • ALL RIGHTS RESERVED
        </p>
      </footer>
    </div>
  );
}
