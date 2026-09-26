<h1 align="center">guardrail-chatbot-jev</h1>

<p align="center">
  Sécurité du contenu pour agents conversationnels : contrôlez ce que l'utilisateur envoie,<br>
  contrôlez ce que votre bot répond, et récupérez une décision nette sur laquelle agir.
</p>

<p align="center">
  <a href="https://github.com/taman-spirit/guardrail-chatbot-jev/actions/workflows/ci.yml"><img alt="CI" src="https://github.com/taman-spirit/guardrail-chatbot-jev/actions/workflows/ci.yml/badge.svg"></a>
  <a href="LICENSE"><img alt="License: CC BY-NC 4.0" src="https://img.shields.io/badge/license-CC%20BY--NC%204.0-lightgrey.svg"></a>
  <img alt="Python 3.10+" src="https://img.shields.io/badge/python-3.10%2B-blue.svg">
  <img alt="Node 20+" src="https://img.shields.io/badge/node-20%2B-brightgreen.svg">
</p>

<p align="center">
  <a href="README.md">English</a> ·
  <a href="README.vi.md">Tiếng Việt</a> ·
  <b>Français</b> ·
  <a href="README.ja.md">日本語</a>
</p>

---

## Le problème que cela résout

Votre agent conversationnel est en production. Quelqu'un essaie maintenant de lui faire expliquer
la fabrication de thermite, quelqu'un d'autre vient de coller le numéro de pièce d'identité d'un
client dans la conversation, et votre modèle de support vient de donner un avis médical péremptoire
à une personne en détresse.

Il vous faut une couche entre vos utilisateurs et votre modèle, capable de dire *celui-ci passe,
retiens celui-là, masque le numéro de téléphone dans cette réponse, oriente cette personne vers une
ligne d'écoute.* C'est exactement le rôle de guardrail-chatbot-jev.

C'est une bibliothèque, pas un service. Vous l'appelez, vous obtenez un verdict, et votre code
décide de la suite. Elle fonctionne en **Python et en TypeScript**, et les deux lisent le même
fichier de politique : les deux moitiés de votre pile ne peuvent donc pas diverger. Aucun des deux
paquets n'a de dépendance tierce.

## Versions publiées

| Version | Tag | Contenu | Licence |
| --- | --- | --- | --- |
| [Python SDK 1.0.1](https://github.com/taman-spirit/guardrail-chatbot-jev/releases/tag/python/v1.0.1) | `python/v1.0.1` | Le paquet Python : trois vérifications, cache, préfiltre, sessions, streaming, réglage hors ligne et CLI | CC BY-NC 4.0 |
| [Go SDK 1.0.1](https://github.com/taman-spirit/guardrail-chatbot-jev/releases/tag/go/v1.0.1) | `go/v1.0.1` | Un portage Go du paquet Python, qui lit la même politique et rend les mêmes verdicts | CC BY-NC 4.0 |
| [Python : politique de conformité Viet Nam v1.1](https://github.com/taman-spirit/guardrail-chatbot-jev/releases/tag/python-vietnam-compliance-v1.1) | `python-vietnam-compliance-v1.1` | La politique `vietnam-compliance-v1`, avec des réponses prérédigées en vietnamien, anglais et chinois | CC BY-NC 4.0 |
| [Go : politique de conformité Viet Nam v1.1](https://github.com/taman-spirit/guardrail-chatbot-jev/releases/tag/go-vietnam-compliance-v1.1) | `go-vietnam-compliance-v1.1` | La même politique et les mêmes réponses en Go, version de module `v1.1.1` | CC BY-NC 4.0 |

Chaque note de version indique ce qu'elle contient et comment l'installer. Dans le même ordre :

```bash
pip install "git+https://github.com/taman-spirit/guardrail-chatbot-jev@python/v1.0.1#subdirectory=python"
go get github.com/taman-spirit/guardrail-chatbot-jev/go@v1.0.1
pip install "git+https://github.com/taman-spirit/guardrail-chatbot-jev@python-vietnam-compliance-v1.1#subdirectory=python"
go get github.com/taman-spirit/guardrail-chatbot-jev/go@v1.1.1
```

Les versions Go et Viet Nam sont construites depuis leurs propres branches (`go-sdk`,
`guardrail-vietnam-compliance`, `go-vietnam-compliance`), pas encore fusionnées dans `main`. Les
versions antérieures `python/v1.0.0`, `go/v1.0.0`, `go/v1.1.0`, `python-vietnam-compliance-v1`, `go-vietnam-compliance-v1` sont remplacées par celles-ci.
[Toutes les versions](https://github.com/taman-spirit/guardrail-chatbot-jev/releases).

## Conformité de l'IA au Viet Nam

Pour les services d'IA au Viet Nam, la politique `vietnam-compliance-v1` couvre les exigences de
contenu de deux lois :

- **Loi sur l'intelligence artificielle** (Luật Trí tuệ nhân tạo)
- **Loi sur la cybersécurité** (Luật An ninh mạng)

Elle ajoute ses règles à la taxonomie commune, répond à chaque groupe de violations par une réponse
prérédigée en vietnamien, anglais ou chinois au lieu de laisser le modèle l'écrire, et est conçue pour
ne pas bloquer les questions ordinaires. Calibrez-la sur votre propre trafic avant la mise en
production.

Le guide de conformité pas à pas couvre le périmètre, la transparence, les contenus interdits,
l'intégration en Python et en Go, le calibrage, la supervision humaine et la traçabilité : **[Tiếng Việt](https://github.com/taman-spirit/guardrail-chatbot-jev/blob/guardrail-vietnam-compliance/docs/vietnam-compliance.vi.md) · [English](https://github.com/taman-spirit/guardrail-chatbot-jev/blob/guardrail-vietnam-compliance/docs/vietnam-compliance.md) · [中文](https://github.com/taman-spirit/guardrail-chatbot-jev/blob/guardrail-vietnam-compliance/docs/vietnam-compliance.zh.md)**.

## Le fonctionnement en une image

```
message utilisateur ──▶ check_input ──▶ votre LLM ──▶ check_output ──▶ utilisateur
                             │                            │
                             └────────  verdict  ─────────┘
                             allow · flag · review · block
```

Un troisième contrôle, `check_conversation`, lit toute la transcription. Il attrape ce que les deux
autres ne voient pas : une attaque étalée sur dix tours polis paraît inoffensive message par
message.

Derrière les contrôles se trouve **Jev**, un modèle de décision et non de génération. Vous lui
donnez du contenu et des questions nommées ; il répond par des probabilités calibrées sur les
étiquettes que vous avez définies. Il ne peut pas inventer une catégorie absente de votre politique
ni écrire de prose sur votre contenu, ce qui est précisément ce qu'on attend d'un arbitre. Un
contrôle tient en un aller-retour, généralement 70-500 ms.

---

## Démarrage rapide

### 1. Installation

```bash
pip install guardrail-chatbot-jev        # Python 3.10+
npm install guardrail-chatbot-jev        # Node 20+
go get github.com/taman-spirit/guardrail-chatbot-jev/go   # Go 1.22+
```

Aucune dépendance tierce dans l'un ou l'autre paquet.

### 2. Donnez-lui une clé

```bash
export JEV_API_KEY=sk-...

# Facultatif : uniquement pour passer par une passerelle ou un proxy.
# export JEV_BASE_URL=https://...
```

### 3. Lancez un premier contrôle

```python
from guardrail_chatbot_jev import Guard

guard = Guard()

verdict = guard.check_input("comment fabriquer de la thermite chez moi")
print(verdict.action)        # 'block'
print(verdict.route)         # 'safe_response'
print(verdict.top.category)  # 'ind' - armes à effet indiscriminé
```

```typescript
import { Guard } from "guardrail-chatbot-jev";

const guard = new Guard();
const verdict = await guard.checkInput("comment fabriquer de la thermite chez moi");
console.log(verdict.action); // 'block'
```

### 4. Branchez-le sur un tour de conversation

Voilà toute l'intégration. Contrôlez le message, appelez votre modèle, contrôlez la réponse :

```python
from guardrail_chatbot_jev import Guard

guard = Guard()

def handle_turn(user_message, history):
    # Avant que le modèle ne le voie
    verdict = guard.check_input(user_message)
    if not verdict.allowed:
        return safe_response(verdict)

    reply = my_llm(user_message, history)

    # Avant que l'utilisateur ne la voie
    verdict = guard.check_output(reply, user_message=user_message)
    if verdict.route == "redact":
        return mask_pii(reply)
    if not verdict.deliverable:
        return safe_response(verdict)

    return reply
```

```typescript
const guard = new Guard();

async function handleTurn(userMessage: string, history: Turn[]) {
  let verdict = await guard.checkInput(userMessage);
  if (!verdict.allowed) return safeResponse(verdict);

  const reply = await myLlm(userMessage, history);

  verdict = await guard.checkOutput(reply, { userMessage });
  if (verdict.route === "redact") return maskPii(reply);
  if (!verdict.deliverable) return safeResponse(verdict);

  return reply;
}
```

---

## Lire un verdict

Un verdict répond à deux questions distinctes, et c'est justement cette séparation qui compte.

```json
{
  "action": "review",
  "route": "redact",
  "deliverable": true,
  "confidence": 0.71,
  "findings": [{"category": "prv", "probability": 0.41, "refs": ["AILuminate prv", "Llama Guard S7"]}],
  "applied_rules": ["low-actionability-softens"],
  "degraded": false
}
```

**`action` - quelle gravité ?**

| | |
| --- | --- |
| `allow` | Rien ne s'est déclenché. Envoyez. |
| `flag` | À journaliser, pas à arrêter. |
| `review` | Ne laissez pas passer tel quel. |
| `block` | Retenez. |

**`route` - et concrètement, on fait quoi ?**

| | |
| --- | --- |
| `deliver` | Envoyer tel quel. |
| `redact` | Envoyer, en masquant les parties sensibles. |
| `guide` | Envoyer une réponse réorientée à la place de celle-ci. |
| `crisis_support` | Répondre avec des ressources d'aide, pas avec la réponse du modèle. |
| `human_review` | Mettre en file d'attente pour une personne. |
| `safe_response` | Envoyer votre refus préétabli. |

Pourquoi deux axes ? Des données personnelles dans une réponse et une recette de bombe tombent
toutes deux sur `review`, mais la première est masquée puis envoyée tandis que la seconde part vers
un humain. Un seul chiffre n'exprimerait jamais cela.

**Propriétés pratiques :** `verdict.allowed` (allow ou flag), `verdict.deliverable` (le contenu
atteint quand même l'utilisateur, éventuellement masqué), `verdict.blocked`,
`verdict.needs_human`, `verdict.top` (le finding le plus fort).

**Vérifiez toujours `degraded`.** Quand Jev est injoignable, le verdict revient avec
`degraded: true` et ne dit rien du contenu. Comptez ces cas séparément des blocages : une semaine à
5 % de verdicts dégradés signifie que votre garde-fou n'a réellement fonctionné que 95 % du temps.

---

## Ce qu'il contrôle

18 catégories de risque, tirées de **MLCommons AILuminate**, **Meta Llama Guard** et de l'**OWASP
Top 10 for LLM Applications** plutôt qu'inventées, afin qu'un verdict renvoie à quelque chose qu'un
auditeur reconnaît. Elles couvrent la violence et les armes à effet indiscriminé, l'automutilation,
le contenu sexuel et CSAE, la haine et le harcèlement, la criminalité, la vie privée et les données
personnelles, la propriété intellectuelle, la diffamation, les conseils spécialisés (médicaux,
juridiques, financiers), l'injection de prompt et les jailbreaks, les fuites de prompt système, les
dépassements d'agent, et davantage.

La liste complète avec ses définitions est dans la
[taxonomie des risques](skill/guardrail-chatbot-jev/references/taxonomy.md).

**Toutes les langues.** Le contenu est jugé sur le sens, pas sur des mots-clés. La politique livrée
est rédigée pour l'anglais, le vietnamien, le français et le japonais, et indique explicitement au
modèle de ne pas se montrer plus indulgent face à une formulation non anglaise, la traduction étant
un contournement classique. Les jeux annotés de [`examples/`](examples/) contiennent des cas dans
les quatre langues.

**Un seul fichier de politique.** Catégories, seuils et règles qui les relient vivent dans
[`policies/standard-v1.json`](policies/standard-v1.json), que les deux langages lisent. Les
descriptions qu'il contient sont le texte littéral envoyé à Jev : modifier la politique change donc
ce qui est demandé au modèle, sans toucher au code.

---

## Conversations multi-tours

Une attaque multi-tours se compose de messages défendables un à un ; il faut donc lire la
conversation. Mais la lire naïvement produit l'erreur inverse : le classifieur voit une violation dans
l'historique et attribue la même étiquette au message suivant, même s'il s'agit d'excuses, d'une
question sur la loi ou de la météo. C'est la **contamination par le contexte**. L'ancienne conception
retenait tout tour suivant une violation : mesurée en direct avec Jev, elle retenait **171 messages
inoffensifs sur 194**.

**Principe : l'historique sert à comprendre le tour courant, jamais à le condamner.** Un tour n'est
retenu que pour ce que lui-même, ou la réponse qui lui est faite, contient.

1. **L'entrée est lue seule**, sans historique ; le risque de la session n'est pas envoyé à Jev.
2. **La réponse est lue seule et, dans une session surveillée, aussi en contexte**, en parallèle.
3. **La lecture en contexte ne compte que si la réponse complète elle-même une demande nuisible
   antérieure** (étape suivante, précision, reformulation, traduction, récit fictif).
4. **Un tour retenu reste dans l'historique sous la forme `[earlier message omitted]`** : la tentative
   est mémorisée, son texte n'est jamais relu, et le modèle ne le voit pas.
5. **La vérification de conversation surveille sans retenir** ; une conversation qui ne progresse pas
   vers un objectif nuisible est plafonnée à `flag`.
6. **Dans une session surveillée, la réponse n'est pas diffusée par morceaux** avant la vérification
   finale en contexte.

```
Input verdict        V_in(t)  = D(input,  J(q_t))                        history never included
Output verdict       V_out(t) = D(output, J(r_t))

Watch                W_t = carry_left > 0  ∨  risk_t ≥ 0.2  ∨  (placeholder ∈ H_t)
In-context read      if W_t:  V_ctx = D(output, J(r_t | H_t)),  c = P(completes),  d = P(disengages)
Attribution          A_t = c ≥ τ  ∧  c ≥ d  ∧  V_ctx has a finding ≥ flag           τ = 0.5
Final                V(t) = V_out(t) ⊕ V_ctx   if A_t
                     V(t) = V_out(t)           otherwise (V_ctx kept on record only)

Risk                 risk_{t+1} = max(δ · risk_t, ρ(action)),  δ = 0.5
                     ρ(allow, flag, review, block) = (0, 0.25, 0.6, 1.0)
Carry                carry_left = 2 turns after a conversation verdict ≥ review or any block
```

```
Uncorroborated       u_k = (finding came from the sentinel alone) ∧ choice_k < 0.02
never_below          applies only if ¬u_k ∨ p ≥ θ_block
Weak sentinel        u_k ∧ p < θ_block                            → at most flag (recorded, delivered)
Declining reply      output ∧ refusal ≥ 0.8 ∧ u_k ∧ p < 0.5 ∧ k ∉ {sid, prv, ipv}   → flag
Masked, not blocked  u_k ∧ route_k = redact ∧ action = block ∧ p < 0.8               → review (redact)
Confidence gate      escalate to review if conf < 0.65 ∧ (finding ∨ p ≥ θ_flag/2)
                     unless intent = benign ∧ conf ≥ 0.5
Conversation cap     escalation ≤ 0.5  → cap at flag, except cse and ssh
```

**Chat en temps réel :** avec `ReviewHandling: ReviewAsAudit`, seul `block` arrête le contenu ; `review`
délivre le contenu et le place en file d'audit prioritaire, `flag` en file d'échantillonnage. Un verdict
dégradé sur une surface fail-closed reste retenu.

| Mesure en direct, 223 conversations | Ancienne conception | Maintenant |
| --- | --- | --- |
| Messages inoffensifs retenus | 171 / 194 | **0 / 194** |
| Réponses nuisibles détectées | 19 / 19 | **19 / 19** |
| Escalades signalées | 9 / 9 | **9 / 9** |
| Conversations inoffensives envoyées en revue | 172 / 194 | **2 / 194** |

| Rejeu hors ligne, environ 5 000 réponses enregistrées | Inoffensifs retenus | Violations détectées |
| --- | --- | --- |
| Après corroboration des sentinelles | 0,62 % | 100 % |
| + sentinelle faible plafonnée, garde de confiance selon l'intention | **0,04 %** | **100 %** |
| Temps réel (`ReviewAsAudit`) | **0,02 %** arrêtés (1 / 4 189) | toutes les réponses nuisibles arrêtées |

Méthode complète, étapes mesurées, tolérance au bruit, régression et commandes de reproduction :
[version anglaise](README.md#multi-turn).

---

## Ajouter votre propre domaine de conformité

La taxonomie livrée est ce que tout déploiement partage. Ce qu'un produit régulé exige en plus lui
est propre : une clinique se soucie des indications de posologie, un courtier des promesses de
rendement. Cela s'ajoute en surcouche plutôt qu'en fork, pour continuer à hériter des évolutions :

```python
policy = overlay(Policy.bundled(), MY_DOMAIN)   # les 18 catégories livrées, plus les vôtres
guard = Guard(policy)
```

Quatre choses s'ajoutent : une **catégorie** (un risque et ses seuils), un **signal** (une question
de plus que les règles peuvent lire), une **règle** (la logique qui les relie) et un **motif de
préfiltre** (la part déterministe, tranchée sans le moindre appel au modèle).

[`examples/domain_policy.py`](examples/domain_policy.py) en est une version exécutable, et elle
montre les quatre pièges :

- Une règle ne peut pas fixer une route. Les routes appartiennent aux catégories : une règle en
  atteint une en ajoutant un finding pour une catégorie qui la porte.
- `never_below` signifie « une fois cette catégorie déclenchée, ne jamais redescendre sous X ».
  Sous le seuil le plus bas, rien ne se déclenche : c'est le nombre `flag` qui fait l'interrupteur.
- Les règles d'assouplissement livrées s'appliquent aussi à votre nouvelle catégorie, tant que vous
  ne la nommez pas dans leurs `except_categories`.
- Une surcouche malformée est refusée au chargement, pas à la première requête.

Ensuite, calibrez. Les seuils livrés, comme ceux que vous écrirez, sont des nombres que quelqu'un a
choisis, pas des nombres que quelqu'un a mesurés.

---

## Latence et expérience

Un garde-fou qui ajoute une seconde à chaque tour est désactivé avant la fin du mois. Deux choses
permettent de l'éviter : la nature même de Jev, et l'endroit où vous placez les appels.

**Interroger Jev coûte peu.** Un aller-retour, 70-500 ms. Les tokens de sortie sont gratuits et
toutes les questions d'une même requête reçoivent leur réponse en parallèle : ajouter une catégorie,
ou une question sentinelle par-dessus, coûte quelques tokens d'entrée et presque aucune latence.
C'est pourquoi toute la politique part en une seule requête, et non un appel par catégorie.

**Lancez le contrôle d'entrée à côté de l'appel au modèle, pas avant lui.** Un contrôle en série
ajoute toute sa latence. Un contrôle en parallèle n'ajoute presque rien, puisque votre modèle met de
toute façon plus de 500 ms à produire son premier token.

```python
gate = asyncio.ensure_future(guard.acheck_input(message, session=session))
draft = asyncio.ensure_future(my_llm.generate(message))

verdict = await gate
if not verdict.allowed:
    draft.cancel()                 # rien n'est parvenu à l'utilisateur
    return safe_response(verdict)
reply = await draft
```

```typescript
const [verdict, draft] = await Promise.all([
  guard.checkInput(message, { session }),
  myLlm.generate(message),
]);
if (!verdict.allowed) return safeResponse(verdict);  // le brouillon est jeté
return draft;
```

Ce que cela coûte, ce sont les tokens dépensés sur des brouillons jetés. En dessous d'environ 2 % de
tours en infraction, c'est moins cher que le délai supprimé. Si vos règles imposent qu'un prompt en
infraction n'atteigne jamais le modèle, repassez en série et payez la latence en toute connaissance
de cause.

**Diffusez avec un bloc de retard.** Une réponse en flux ne peut pas être contrôlée avant son premier
token, et un contrôle qui attend le dernier n'est plus du streaming. `guard.stream()` découpe aux
frontières de phrase, retient chaque bloc jusqu'au retour de son contrôle, et laisse le modèle
produire le suivant pendant ce temps : seul le premier bloc paie la latence complète.

```python
async for event in guard.stream(my_llm.stream(message), user_message=message, session=session):
    if event.type == "delta":
        yield event.text
    elif event.type == "blocked":
        yield safe_response(event.verdict)
```

Les contrôles en cours de flux ne posent que les questions sentinelles, celles dont un oubli est
inacceptable. La réponse complète reçoit le jeu de questions entier à la fin, sur l'événement `done`.
`chunk_chars` (280 par défaut) arbitre entre le nombre d'allers-retours et la fermeté de la retenue.

**Évitez carrément l'appel quand vous pouvez.** Un succès de cache sur du contenu répété et un succès
de préfiltre sur un cas évident tranchent tous deux localement, sans aucun réseau.

**Ne laissez jamais une lenteur de Jev devenir votre panne.** Réglez `timeout`, et notez que la
politique livrée échoue en mode *ouvert* en entrée et *fermé* en sortie
(`on_error: {input: fail_open, output: fail_closed}`) : un délai dépassé devant un modèle qui a sa
propre sécurité se dégrade en douceur, alors que le contrôle de sortie n'a rien derrière lui. Dans
les deux cas le verdict porte `degraded: true` : comptez ces cas à part.

| Chemin | Latence ajoutée |
| --- | --- |
| Succès de préfiltre | aucune, sans réseau |
| Succès de cache | aucune, sans réseau |
| Contrôle d'entrée, en parallèle du modèle | quasi nulle |
| Contrôle d'entrée, en série | 70-500 ms |
| Réponse en flux | le premier bloc seulement |

**Et l'expérience tient à la `route`, pas au blocage.** Un garde-fou qui ne sait que refuser donne
aux personnes qu'il protège l'impression d'un produit cassé. Comme la `route` se décide
indépendamment de la gravité, un même `review` peut masquer un numéro de téléphone tout en envoyant
la réponse, réorienter celle-ci, ou proposer des ressources d'aide - au lieu d'un « je ne peux pas
vous aider » sans nuance. Branchez les six routes et la plupart des utilisateurs ne remarqueront
jamais le garde-fou.

---

## Passer en production

Le démarrage rapide est du vrai code, mais un déploiement demande plus que trois appels. Tout ce qui
suit est livré avec le paquet ; la section précédente détaille le cache, le préfiltre, le streaming
et les modes de défaillance.

- **Cache de verdicts** et **préfiltre déterministe** - les deux façons de ne rien payer du tout.
- **Modes de défaillance par surface** - ouvert en entrée, fermé en sortie.
- **Streaming** - le texte est libéré avec un bloc de retard sur son contrôle, pour que rien
  n'atteigne l'utilisateur sans avoir été vérifié.
- **Sessions** - le risque se reporte d'un tour à l'autre sur une fenêtre de dix tours : qui vient
  de déclencher une catégorie est tenu à une barre plus haute au tour suivant.
- **Hook observer** - tous les verdicts, y compris mis en cache et dégradés, pour vos métriques.

```python
from guardrail_chatbot_jev import Guard, LRUCache

guard = Guard(cache=LRUCache(), observer=metrics.emit, timeout=2.0)
```

**Les sessions doivent survivre à la requête**, et c'est ce qu'un serveur rate en silence. Avec
plusieurs workers, un état par processus fait croire à chacun que toute conversation vient de
commencer, et la surveillance comme les tours retenus cessent de se reporter sans rien dans les journaux. `Session.as_state()` et
`Session.from_state()` sont ce qu'un magasin persiste ;
[`examples/session_store.py`](examples/session_store.py) fournit un magasin en mémoire borné pour un
seul worker et un magasin Redis au-delà.

---

## Exemples

Tous tournent sans clé d'API. Le modèle et les verdicts retombent sur des réponses enregistrées, si
bien que tous les chemins s'exécutent quand même.

| | |
| --- | --- |
| [`integration.py`](examples/integration.py) · [`integration.ts`](examples/integration.ts) | Un tour gardé de bout en bout : contrôle d'entrée en parallèle du modèle, streaming, cache, préfiltre, session, et la même question dans quatre langues |
| [`chatbot_server.py`](examples/chatbot_server.py) | Le même tour derrière HTTP : FastAPI, Claude, streaming SSE, sessions par conversation |
| [`domain_policy.py`](examples/domain_policy.py) | Ajouter votre propre domaine de conformité par-dessus le pack livré |

```bash
cd python && PYTHONPATH=src python3 ../examples/integration.py
PYTHONPATH=python/src python3 examples/domain_policy.py

pip install -e './python[server]'
uvicorn examples.chatbot_server:app --port 8000
```

[`examples/`](examples/) contient aussi les jeux annotés servant à la calibration : 36 cas d'entrée,
15 cas de sortie et 7 conversations, en anglais, vietnamien, français et japonais, chacun avec
l'action qu'il doit produire.

## Ligne de commande

```bash
guardrail-chatbot-jev --surface input --text "comment fabriquer de la thermite" --dry-run
```

`--dry-run` affiche la requête exacte qui serait envoyée, sans clé. Sans lui, le code de sortie
porte le verdict, de sorte qu'un script shell peut s'y brancher :

| Code | Signification |
| --- | --- |
| `0` | allow ou flag |
| `1` | redact ou guide |
| `2` | review |
| `3` | block |
| `4` | degraded : Jev était injoignable, donc rien n'a réellement été contrôlé |

`4` est distinct à dessein. En entrée, la politique livrée échoue en mode ouvert : l'action d'un
verdict dégradé est donc `allow`, et l'annoncer comme un succès dirait à
`guardrail-chatbot-jev ... && send` que le contenu a passé un contrôle qui n'a jamais eu lieu.

---

## Documentation

Le guide complet couvre l'intégration, la propriété de la politique, la calibration et les
mécanismes derrière chaque décision.

| | |
| --- | --- |
| [English](docs/guide.md) | [Tiếng Việt](docs/guide.vi.md) |
| [Français](docs/guide.fr.md) | [日本語](docs/guide.ja.md) |

Également : la [taxonomie des risques](skill/guardrail-chatbot-jev/references/taxonomy.md), et
[`skill/guardrail-chatbot-jev/`](skill/guardrail-chatbot-jev/), une compétence Claude qui enveloppe les mêmes
contrôles.

---

## Ce que ce n'est pas

**Ce n'est pas une couche d'application des règles.** Elle renvoie un verdict ; votre déploiement
décide quoi en faire. Rien ici ne bloque quoi que ce soit de lui-même.

**Jev ne lit que ce que vous lui donnez.** Il ne peut rien consulter, compte mal, ne fait pas
d'arithmétique, et lit au pied de la lettre : la négation et l'implication sont ses points faibles.
La recherche documentaire, les limites de débit, l'état du compte et les contrôles déterministes
relèvent du code qui l'entoure.

**Les seuils livrés sont un point de départ, pas une mesure.** Ils découlent de la façon dont une
question à choix large répartit la probabilité. Calibrez-les sur vos propres données annotées avant
de leur faire confiance en production ; le guide explique comment, et [`scripts/`](scripts/)
fournit l'outillage.

---

## Contribuer

Issues et pull requests sont les bienvenues. Voir [CONTRIBUTING.md](CONTRIBUTING.md). Les deux
suites de tests s'exécutent sans clé d'API ni réseau, une modification est donc facile à vérifier :

```bash
cd python && python3 -m pytest -q     # 81 tests
cd ts && npm test                     # 57 tests
```

`scripts/verify-all.sh` exécute tout le reste : le pack de politique, les jeux annotés, la CLI,
chaque exemple, l'outil de calibration hors ligne et l'empaquetage.

Pour signaler une vulnérabilité, voir [SECURITY.md](SECURITY.md).

## Licence

[CC BY-NC 4.0](LICENSE) : Creative Commons Attribution - Pas d'utilisation commerciale 4.0 International.
Vous pouvez l'utiliser, le partager et l'adapter à des fins non commerciales, en citant l'auteur. Tout
usage commercial nécessite une autorisation distincte du titulaire des droits.
