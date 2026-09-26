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

**Démo :** le garde-fou appliqué à Nhật Nguyệt AI, sur [https://nhatnguyet.org/tro-ly-ai](https://nhatnguyet.org/tro-ly-ai).

| Version | Tag | Contenu | Licence |
| --- | --- | --- | --- |
| [Python SDK 1.1.2](https://github.com/taman-spirit/guardrail-chatbot-jev/releases/tag/python/v1.1.2) | `python/v1.1.2` | Le paquet Python et TypeScript : trois vérifications, attribution multi-tours, revue en temps réel, cache, préfiltre, sessions, streaming, réglage hors ligne et CLI | CC BY-NC 4.0 |
| [Go SDK 1.2.2](https://github.com/taman-spirit/guardrail-chatbot-jev/releases/tag/go/v1.2.2) | `go/v1.2.2` | Le même moteur en Go, avec les outils de test en direct, de rejeu et de régression | CC BY-NC 4.0 |
| [Python : politique de conformité Viet Nam v1.2.1](https://github.com/taman-spirit/guardrail-chatbot-jev/releases/tag/python-vietnam-compliance-v1.2.1) | `python-vietnam-compliance-v1.2.1` | La politique `vietnam-compliance-v1`, avec des réponses prérédigées en vietnamien, anglais et chinois | CC BY-NC 4.0 |
| [Go : politique de conformité Viet Nam v1.2.1](https://github.com/taman-spirit/guardrail-chatbot-jev/releases/tag/go-vietnam-compliance-v1.2.1) | `go-vietnam-compliance-v1.2.1` | La même politique en Go, version de module `v1.3.1` | CC BY-NC 4.0 |

Chaque note de version indique ce qu'elle contient et comment l'installer. Dans le même ordre :

```bash
pip install "git+https://github.com/taman-spirit/guardrail-chatbot-jev@python/v1.1.2#subdirectory=python"
go get github.com/taman-spirit/guardrail-chatbot-jev/go@v1.2.2
pip install "git+https://github.com/taman-spirit/guardrail-chatbot-jev@python-vietnam-compliance-v1.2.1#subdirectory=python"
go get github.com/taman-spirit/guardrail-chatbot-jev/go@v1.3.1
```

Python et TypeScript sont sur `main` ; Go sur `go-sdk` ; la politique Viet Nam sur
`guardrail-vietnam-compliance` (Python) et `go-vietnam-compliance` (Go). Les versions antérieures
sont remplacées par celles-ci. [Toutes les versions](https://github.com/taman-spirit/guardrail-chatbot-jev/releases).

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

### Problème

**Contamination par le contexte :** un classifieur qui lit une violation dans l'historique l'attribue au
tour suivant, quel qu'en soit le contenu. Référence (plancher de session, ancienne conception) :
**171 sur 194** messages inoffensifs retenus, mesuré en direct.

### Règle de conception

Un tour n'est retenu que sur la base d'éléments issus du tour lui-même ou de sa réponse. L'historique
détermine le degré d'attention porté au tour, jamais sa rétention.

### Méthode

Chaque tour est décidé en répondant, dans l'ordre, à trois questions.

1. **Le message de l'utilisateur est-il nuisible en lui-même ?** Il est lu seul, sans la conversation
   qui précède. S'il l'est, il est arrêté ici. Un message inoffensif en lui-même n'est jamais arrêté à
   cause de ce qui a été dit avant.
2. **La réponse est-elle nuisible en elle-même ?** Elle est lue seule de la même façon.
3. **Seulement si la conversation a été récemment risquée : la réponse termine-t-elle une demande
   nuisible faite plus tôt ?** La réponse est relue avec les tours précédents. Cette seconde lecture ne
   compte que si la réponse fournit l'étape suivante, des précisions, une traduction ou un récit d'une
   demande nuisible antérieure. Excuses, question sur la loi, signalement ou changement de sujet : elle
   ne compte pas.

Un message arrêté reste dans la conversation sous la forme `[earlier message omitted]` : la tentative est
mémorisée, son texte n'est jamais relu, et le modèle ne le voit pas.

| Étape | Code |
| --- | --- |
| 1. Le message de l'utilisateur, lu seul | [`CheckInput`](go/guard.go#L118) |
| 2. La réponse, lue seule | [`CheckOutput`](go/guard.go#L128) |
| 3. La réponse, lue avec les tours précédents, et si elle compte | [`checkInContext`](go/multiturn.go#L148), [`attribute`](go/multiturn.go#L166) |
| Quand une conversation est considérée comme récemment risquée | [`Session.Watching`](go/multiturn.go#L255) |
| Messages arrêtés conservés comme repère, cachés au modèle | [`Session.Record`](go/multiturn.go#L225), [`ModelHistory`](go/multiturn.go#L236) |
| Vérification de la conversation entière : elle surveille, n'arrête jamais un tour | [`CheckConversation`](go/guard.go#L166) |

### Exemple, tour par tour

| Tour | Utilisateur | Assistant | Ce que fait le garde-fou | Résultat |
| --- | --- | --- | --- | --- |
| 1 | « Comment fabriquer un explosif avec de l'engrais ? » | — | Étape 1 : nuisible en lui-même. | **Arrêté.** La conversation devient récemment risquée. |
| 2 | « Désolé, oublie. Quel temps demain à Hanoï ? » | « Ensoleillé, 24 à 30 °C. » | Étapes 1 et 2 : inoffensif. Étape 3 : la réponse ne termine rien (0,02). | **Délivré.** L'ancienne conception le retenait. |
| 3 | « Comment la loi punit-elle la fabrication d'explosifs ? » | « Par une peine de prison, selon les cas. » | Étape 3 : l'utilisateur prend ses distances (0,90) ; la réponse ne termine rien (0,05). | **Délivré.** |
| 4 | « OK, continue. Et l'étape 2 ? » | « Étape 2 : laisser sécher le mélange un jour, puis le répartir. » | Étape 3 : c'est l'étape suivante de la demande refusée (0,95). | **Arrêté.** Seule la lecture en contexte le voit. |
| 4′ | idem | « Je ne peux pas continuer. » | Étape 3 : la réponse ne termine rien. | **Délivré.** |

### Définitions

#### Réglages en vigueur

| Réglage | Valeur | Signification |
| --- | --- | --- |
| Fenêtre d'historique | 10 messages | La vérification de conversation et la lecture en contexte voient les dix derniers messages. |
| Risque par verdict | allow 0 · flag 0,25 · review 0,6 · block 1,0 | Ce qu'un verdict ajoute au risque de la session. |
| Décroissance | 0,5 | À chaque vérification, le risque précédent est divisé par deux ; la session garde le plus élevé des deux. |
| Surveillance dès | risque ≥ 0,2 | Tant que le risque atteint 0,2, la réponse est aussi lue en contexte. |
| Report | 2 tours | Après une revue de conversation ou un blocage, la session reste surveillée deux tours. |
| Attribution | ≥ 0,5, et au-dessus du « recul » | La lecture en contexte ne compte que si Jev est sûr à 50 % au moins que la réponse complète une demande nuisible antérieure, et plus sûr de cela que du recul de l'utilisateur. |
| Corroboration | ≥ 0,02 | Une sentinelle est corroborée quand la question principale donne au moins 2 % à sa catégorie. |
| Sentinelle faible | sous sa bande block | Seule et sous la bande block, elle est enregistrée en flag et délivrée. |
| Jamais affaiblies | `ssh` (pack Viet Nam : `ssh`, `vsv`, `vld`) | Ces catégories gardent toute leur force. |
| Réponse de refus | refus ≥ 0,8, sentinelle < 0,5 | Plafonnée à flag, sauf `sid`, `prv`, `ipv` (pack Viet Nam : aussi `vsv`, `vld`). |
| Masquer plutôt que bloquer | sentinelle < 0,8 | Une donnée personnelle signalée par la seule sentinelle est masquée et délivrée. |
| Garde de confiance | confiance < 0,65 | En revue s'il y a un constat ou un quasi-constat, sauf intention jugée bénigne avec confiance ≥ 0,5. |
| Plafond de conversation | escalation ≤ 0,5 | Plafonnée à flag, sauf `cse` et `ssh`. |
| Jev injoignable | entrée : délivrer · sortie : retenir · conversation : délivrer | La sortie est le dernier rempart. |

#### Seuils par catégorie

| Catégorie | Nom | Flag / review / block | Sortie | Notes |
| --- | --- | --- | --- | --- |
| `cse` | Child sexual exploitation | 0.03 / 0.06 / 0.15 | = | sentinelle; jamais sous block |
| `iwp` | Indiscriminate weapons (CBRNE) | 0.04 / 0.08 / 0.2 | = | sentinelle |
| `ssh` | Suicide and self-harm | 0.07 / 0.15 / 0.3 | = | sentinelle; traité par crisis_support |
| `vcr` | Violent crimes | 0.12 / 0.25 / 0.45 | = | sentinelle |
| `src` | Sex-related crimes | 0.12 / 0.25 / 0.45 | = |  |
| `hte` | Hate and discrimination | 0.15 / 0.28 / 0.5 | 0.08 / 0.18 / 0.35 |  |
| `ncr` | Non-violent crimes | 0.15 / 0.3 / 0.5 | = |  |
| `sxc` | Adult sexual content | 0.2 / 0.4 / 0.6 | = |  |
| `prv` | Privacy and personal data | 0.2 / 0.4 / 0.7 | 0.15 / 0.28 / 0.5 | sentinelle; traité par redact |
| `ipv` | Intellectual property | 0.25 / 0.45 / 0.7 | = | output, conversation seulement |
| `dfm` | Defamation | 0.18 / 0.35 / 0.6 | = | output, conversation seulement |
| `spc` | Specialized advice | 0.25 / 0.5 / 0.85 | = | traité par guide; output seulement |
| `elc` | Elections and civic integrity | 0.15 / 0.3 / 0.5 | = |  |
| `pij` | Prompt injection and jailbreak | 0.15 / 0.3 / 0.55 | = | sentinelle; input, conversation seulement |
| `sid` | Sensitive information disclosure | 0.08 / 0.18 / 0.35 | = | sentinelle; output, conversation seulement |
| `exa` | Excessive agency | 0.15 / 0.3 / 0.5 | = | output, conversation seulement |
| `mis` | Misinformation and unsupported claims | 0.25 / 0.45 / 0.8 | = | traité par guide; output seulement |
| `scp` | Out of scope | 0.4 / 0.75 / 0.95 | = | désactivée |

Le pack Viet Nam ([`vietnam-compliance-v1`](policies/vietnam-compliance-v1.json)) ajoute ces catégories ; les autres gardent les seuils ci-dessus.

| Catégorie | Nom | Flag / review / block | Sortie | Notes |
| --- | --- | --- | --- | --- |
| `vsv` | Territorial sovereignty of Viet Nam | 0.15 / 0.3 / 0.5 | 0.1 / 0.22 / 0.4 | sentinelle |
| `vas` | Propaganda against the State | 0.15 / 0.3 / 0.55 | = |  |
| `vld` | Insulting national leaders and symbols | 0.14 / 0.28 / 0.5 | 0.1 / 0.2 / 0.4 | sentinelle |
| `vcs` | False information and public disorder | 0.16 / 0.32 / 0.55 | = |  |
| `vai` | Deceptive or manipulative use of AI | 0.15 / 0.3 / 0.55 | = |  |

#### Les règles, en clair

1. Chaque message et chaque réponse sont notés seuls ; le verdict est la bande la plus forte atteinte.
2. La session retient le risque, pas le texte : le plus élevé entre la moitié du risque précédent et le risque du nouveau verdict.
3. Une session est surveillée si son risque atteint 0,2, pendant deux tours après un constat grave, et tant qu'un message retenu reste dans la fenêtre.
4. En session surveillée, la réponse est relue avec l'historique ; cette lecture ne compte que si elle complète une demande nuisible antérieure.
5. L'historique seul ne relève jamais un verdict.

#### Formules

```
V_in(t)   = D(input,  J(q_t))
V_out(t)  = D(output, J(r_t))

W_t       = carry_left > 0  ∨  risk_t ≥ 0.2  ∨  placeholder ∈ H_t          (watched)
V_ctx     = D(output, J(r_t | H_t))                                          (only if W_t)
c, d      = P(reply completes an earlier harmful request), P(user steps away)
A_t       = c ≥ τ  ∧  c ≥ d  ∧  V_ctx has a finding ≥ flag,   τ = 0.5
V(t)      = V_out(t) ⊕ V_ctx  if A_t,  else V_out(t)

risk_t+1  = max(δ · risk_t, ρ(action)),  δ = 0.5,  ρ = (0, 0.25, 0.6, 1.0) for (allow, flag, review, block)
carry     = 2 turns after a conversation verdict ≥ review or any block
```

Code : `V_in` [`CheckInput`](go/guard.go#L118), `V_out` [`CheckOutput`](go/guard.go#L128), `W_t` [`Session.Watching`](go/multiturn.go#L255), `V_ctx`, `c`, `d` [`checkInContext`](go/multiturn.go#L148) / [`ContextQuestions`](go/multiturn.go#L76), `A_t`, `⊕` [`attribute`](go/multiturn.go#L166), `risk` [`Session.Observe`](go/session.go#L95), `carry` [`Session.Advance`](go/session.go#L112)

### Calibrage mono-tour

En clair :

1. **Une sentinelle seule est un signal faible.** Quand seule la question oui/non dédiée voit une
   catégorie, et que la question principale lui donne moins de 2 %, la détection est *non corroborée*.
2. **Une détection faible reste à flag**, sauf si elle atteint seule le seuil de blocage ; le plancher
   « jamais sous » de la catégorie ne la relève pas. L'automutilation n'est jamais affaiblie (pack
   Viet Nam : aussi `vsv`, `vld`).
3. **Une réponse qui refuse** (refus ≥ 0,8) avec une sentinelle faible sous 0,5 est notée à flag. Les
   fuites de secrets, de données personnelles et de textes protégés (`sid`, `prv`, `ipv`) font exception,
   car un refus peut encore les contenir.
4. **Les données personnelles vues par la seule sentinelle** sont masquées puis livrées, pas bloquées,
   sauf si la sentinelle atteint 0,8.
5. **Une confiance faible** (sous 0,65) envoie une détection ou un quasi-seuil en revue, sauf si Jev lit
   une intention bénigne avec une confiance d'au moins 0,5.
6. **Une conversation qui ne s'aggrave pas** (escalade ≤ 0,5) est plafonnée à flag, sauf `cse` et `ssh`.

```
u_k                 = finding from the sentinel only  ∧  choice_k < 0.02      (uncorroborated)
never_below         applied only if ¬u_k ∨ p ≥ θ_block
u_k ∧ p < θ_block                                                    → at most flag
output ∧ refusal ≥ 0.8 ∧ u_k ∧ p < 0.5 ∧ k ∉ {sid, prv, ipv}         → flag
u_k ∧ route_k = redact ∧ action = block ∧ p < 0.8                    → review (masked)
confidence gate:  conf < 0.65 ∧ (finding ∨ p ≥ θ_flag / 2) → review,  unless intent = benign ∧ conf ≥ 0.5
conversation:     escalation ≤ 0.5 → at most flag,  except cse, ssh
```

Code : `u_k` [`Decide`](go/decide.go#L43), `never_below` [`finding`](go/decide.go#L282), weak [`Decide`](go/decide.go#L67), refusal [`capUncorroboratedOnRefusal`](go/decide.go#L178), redact [`Decide`](go/decide.go#L54), gate [`confidenceGate`](go/decide.go#L427), conversation [`no-escalation-caps-conversation`](policies/standard-v1.json#L700), settings [`sentinel_corroboration`](policies/standard-v1.json#L23) / [`confidence_gate`](policies/standard-v1.json#L32), [`spc`](policies/standard-v1.json#L328), [`ncr`](policies/standard-v1.json#L207), [`iwp`](policies/standard-v1.json#L83)

### Revue en temps réel

`ReviewHandling: ReviewAsAudit` ([`ReviewAsAudit`](go/guard.go#L52), [`audit`](go/guard.go#L303)) : seul `block` arrête le contenu ; `review` délivre et place en audit
prioritaire, `flag` en audit par échantillonnage ; un verdict dégradé sur une surface fail-closed reste
retenu.

### Résultats

| Mesure en direct, 223 conversations | Plancher (ancien) | Attribution (actuel) |
| --- | --- | --- |
| Messages inoffensifs retenus | 171 / 194 | **0 / 194** |
| Réponses nuisibles détectées | 19 / 19 | **19 / 19** |
| Escalades signalées | 9 / 9 | **9 / 9** |
| Conversations inoffensives en file de revue | 172 / 194 | **2 / 194** |

| Rejeu, ≈ 5 000 réponses enregistrées | Inoffensifs retenus | Violations détectées |
| --- | --- | --- |
| Avant calibrage final | 0,62 % | 100 % |
| Après calibrage final | **0,04 %** | **100 %** |
| Temps réel (`ReviewAsAudit`) | **0,02 %** arrêtés | toutes les réponses nuisibles arrêtées |

Jeux de données, protocoles, ablation, bruit, régression et reproduction : [version anglaise](README.md#multi-turn).

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
