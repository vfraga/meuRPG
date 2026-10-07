// Finding U14-3 in review/unit-14-web-maps.md
import { TestBed } from '@angular/core/testing';
import { ActivatedRoute, convertToParamMap, provideRouter } from '@angular/router';
import { BehaviorSubject } from 'rxjs';

import { GalleryClient, type GalleryListing } from '../../core/images/gallery-client';
import {
  FakeGalleryClient,
  FakeImageUploader,
  galleryImage,
  galleryUsage,
} from '../../core/images/gallery-testing';
import { ImageUploader } from '../../core/images/image-uploader';
import { GalleryPage } from './gallery';

const flush = () => new Promise((resolve) => setTimeout(resolve, 0));

describe('Review14 U14-3: gallery ignores stale answers after the campaign param changes', () => {
  let params: BehaviorSubject<ReturnType<typeof convertToParamMap>>;
  let gallery: FakeGalleryClient;
  let uploader: FakeImageUploader;

  beforeEach(() => {
    params = new BehaviorSubject(convertToParamMap({ id: 'camp-A' }));
    TestBed.configureTestingModule({
      imports: [GalleryPage],
      providers: [
        provideRouter([]),
        { provide: ActivatedRoute, useValue: { paramMap: params } },
        { provide: GalleryClient, useClass: FakeGalleryClient },
        { provide: ImageUploader, useClass: FakeImageUploader },
      ],
    });
    gallery = TestBed.inject(GalleryClient) as unknown as FakeGalleryClient;
    uploader = TestBed.inject(ImageUploader) as unknown as FakeImageUploader;
  });

  function deferred() {
    let resolve!: (v: GalleryListing) => void;
    const promise = new Promise<GalleryListing>((r) => (resolve = r));
    return { promise, resolve };
  }

  it('shows campaign B images when A answers after B', async () => {
    const imgA = galleryImage('img-a', 'Imagem de A', { campaignId: 'camp-A' });
    const imgB = galleryImage('img-b', 'Imagem de B', { campaignId: 'camp-B' });
    const a = deferred();
    const b = deferred();

    gallery.listResult = a.promise;
    const fixture = TestBed.createComponent(GalleryPage);
    fixture.detectChanges();
    gallery.listResult = b.promise;
    params.next(convertToParamMap({ id: 'camp-B' }));
    expect(gallery.calls).toEqual([
      ['list', 'camp-A'],
      ['list', 'camp-B'],
    ]);

    // B answers first, then the slow answer of A arrives last.
    b.resolve({ images: [imgB], usage: galleryUsage([imgB]) });
    await flush();
    a.resolve({ images: [imgA], usage: galleryUsage([imgA]) });
    await flush();
    fixture.detectChanges();

    const names = Array.from(
      (fixture.nativeElement as HTMLElement).querySelectorAll('.card__name'),
    ).map((n) => n.textContent?.trim());
    expect(names).toEqual(['Imagem de B']);
  });

  it('does not upload a file dropped on A into B after navigating', async () => {
    const a = deferred();
    gallery.listResult = a.promise;
    const fixture = TestBed.createComponent(GalleryPage);
    fixture.detectChanges();
    a.resolve({ images: [], usage: galleryUsage() });
    await flush();
    fixture.detectChanges();

    const el = fixture.nativeElement as HTMLElement;
    const input = el.querySelector('input[type="file"]') as HTMLInputElement;
    const files = [
      new File(['1'], 'um.jpg', { type: 'image/jpeg' }),
      new File(['2'], 'dois.jpg', { type: 'image/jpeg' }),
    ];
    Object.defineProperty(input, 'files', { value: files, configurable: true });
    input.dispatchEvent(new Event('change'));
    expect(uploader.pending.map((p) => p.campaignId)).toEqual(['camp-A']);

    // Same component instance moves to campaign B while the queue still holds "dois.jpg".
    params.next(convertToParamMap({ id: 'camp-B' }));
    uploader.pending[0].resolve(galleryImage('up-1', 'um', { campaignId: 'camp-A' }));
    await flush();

    // Whatever was dropped on A must never be sent to B.
    expect(uploader.pending.map((p) => p.campaignId)).not.toContain('camp-B');
  });
});
