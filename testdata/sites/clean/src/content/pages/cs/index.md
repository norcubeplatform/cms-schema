---
_id: 11111111-1111-4111-8111-111111111111
title: Domů
slug: ''
translationKey: home
seo:
  description: Úvodní stránka s dostatečně dlouhým popisem pro vyhledávače, který říká, o čem web je.
  noindex: false
blocks:
  - _component: hero
    id: hero
    title: Vítejte
    lead: Podívejte se na [služby](/sluzby/) nebo [kontakt](/kontakt/).
    image: /src/assets/hero.jpg
    ctas:
      - label: Služby
        href: /sluzby/
        variant: primary
      - label: Dokumentace
        href: https://docs.example.com/
        variant: ghost
        external: true
        rel: noopener
  - _component: faq
    id: faq
    items:
      - question: Kolik to stojí?
        answer: Záleží na rozsahu, viz [ceník](/sluzby/#cenik).
---
