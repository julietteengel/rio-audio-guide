export type Place = {
  id: string;
  name: string;
  category: string;
  neighborhood: string;
  lat: number;
  lon: number;
  city: string;
  distanceMeters: number;
  audioDurationSeconds: number;
  body: string;
  groundedSourceCount: number;
};

export const MOCK_PLACES: Place[] = [
  {
    id: "cristo-redentor",
    name: "Cristo Redentor",
    category: "Monument",
    neighborhood: "Zona Sul",
    lat: -22.9519,
    lon: -43.2105,
    city: "Rio de Janeiro",
    distanceMeters: 820,
    audioDurationSeconds: 135,
    body: "Inaugurée en 1931, la statue du Christ Rédempteur culmine à 710 mètres au sommet du Corcovado. Elle est aujourd'hui l'un des symboles les plus reconnus du Brésil.",
    groundedSourceCount: 1,
  },
];
