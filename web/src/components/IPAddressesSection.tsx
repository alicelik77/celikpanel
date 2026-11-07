import { lazy, Suspense } from 'react';

// The Settings slot of the server's IP addresses (alpha.128, G14): the card
// and its words load in their own chunk when the section is shown.
// Ayarlar'daki IP adresleri yuvası: kart kendi paketinde yüklenir.
const IPAddressesCard = lazy(() => import('./IPAddressesCard').then((module) => ({ default: module.IPAddressesCard })));

export function IPAddressesSection() {
    return (
        <Suspense fallback={null}>
            <IPAddressesCard />
        </Suspense>
    );
}
